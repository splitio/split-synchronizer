package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/splitio/go-split-commons/v10/dtos"
)

// SegmentState is the full state of one segment as stored by the proxy
type SegmentState struct {
	Name         string
	Active       map[string]int64 // key -> change number
	Removed      map[string]int64 // key -> change number of the tombstone
	ChangeNumber int64
}

// Source gives the Writer access to the proxy's complete local state (the Bolt-backed storage).
// Implementations must read the cursor no later than the data it returns (cursor behind data is safe).
type Source interface {
	Flags() ([]dtos.SplitDTO, int64, error)                        // all flags incl. ARCHIVED, and the flag cursor
	RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) // same for rule-based segments
	Segments(visit func(SegmentState) error) error                 // one segment at a time
}

// missingCursor is the stored cursor assumed for a segment that Redis does not know or whose cursor is unparsable
const missingCursor = int64(-2)

// resync rewrites the full local state into Redis: meta check, flags, rule-based segments, then segments one by one.
// The worker holds no mutex while it runs; every Source and client call goes through retry.
func (w *Writer) resync(source Source) error {
	epoch := w.overflowEpochOf()
	w.resyncPass, w.skipsSeen = true, false
	defer func() { w.resyncPass = false }()
	fields, err := w.readMeta()
	if err != nil {
		return fmt.Errorf("reading meta: %w", err)
	}
	if fp, ok := fields[FingerprintField]; ok && fp != w.fingerprint {
		w.enterMismatch()
		return ErrEnvironmentMismatch
	}

	var flags []dtos.SplitDTO
	var flagsCn int64
	err = w.retry("source flags read", func() error {
		var e error
		flags, flagsCn, e = source.Flags()
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the flags from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if err != nil {
		return fmt.Errorf("reading flags from source: %w", err)
	}
	storedFlags, err := w.readHash(w.keys.Unprefixed().FlagData())
	if err != nil {
		return fmt.Errorf("reading stored flags: %w", err)
	}
	var gone []dtos.SplitDTO
	var goneSkipped []string
	if !sourceEmpty(len(flags), flagsCn) {
		gone, goneSkipped = w.unlistedFlags(storedFlags, flags)
	}
	all, removals := withRemovals(flags, gone)
	ents, skipped := w.flagEntities(all, removals)
	skipped = append(skipped, goneSkipped...)
	released := w.releaseGroup(w.keys.FlagCursorField())
	if _, err := w.writeEntities("flag resync", FlagScript, w.keys.FlagKeys(), w.keys.FlagCursorField(), ents, flagsCn, skipped); err != nil {
		w.restoreGroup(w.keys.FlagCursorField(), released)
		return fmt.Errorf("writing flags: %w", err)
	}

	var rbs []dtos.RuleBasedSegmentDTO
	var rbsCn int64
	err = w.retry("source rule-based segments read", func() error {
		var e error
		rbs, rbsCn, e = source.RuleBasedSegments()
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the rule-based segments from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if err != nil {
		return fmt.Errorf("reading rule-based segments from source: %w", err)
	}
	storedRBS, err := w.readHash(w.keys.Unprefixed().RuleBasedSegmentData())
	if err != nil {
		return fmt.Errorf("reading stored rule-based segments: %w", err)
	}
	var goneRBS []dtos.RuleBasedSegmentDTO
	var goneRBSSkipped []string
	if !sourceEmpty(len(rbs), rbsCn) {
		goneRBS, goneRBSSkipped = w.unlistedRuleBasedSegments(storedRBS, rbs)
	}
	allRBS, rbsRemovals := withRemovals(rbs, goneRBS)
	rbsEnts, rbsSkipped := w.rbsEntities(allRBS, rbsRemovals)
	rbsSkipped = append(rbsSkipped, goneRBSSkipped...)
	releasedRBS := w.releaseGroup(w.keys.RuleBasedSegmentCursorField())
	if _, err := w.writeEntities("rule-based segment resync", RuleBasedSegmentScript, w.keys.RuleBasedSegmentKeys(), w.keys.RuleBasedSegmentCursorField(), rbsEnts, rbsCn, rbsSkipped); err != nil {
		w.restoreGroup(w.keys.RuleBasedSegmentCursorField(), releasedRBS)
		return fmt.Errorf("writing rule-based segments: %w", err)
	}

	listed, err := w.resyncSegments(source, fields)
	if err != nil {
		return fmt.Errorf("resyncing segments: %w", err)
	}
	if err := w.dropUnlistedSegments(fields, listed); err != nil {
		return fmt.Errorf("dropping unlisted segments: %w", err)
	}
	w.clearOverflowIfUnchanged(epoch)
	return nil
}

// sourceEmpty reports a source that holds nothing yet (no entities and no change number). Such a source is not evidence
// that the entities Redis holds were removed (T62), so the pass deletes nothing in that group.
func sourceEmpty(n int, cn int64) bool {
	return n == 0 && cn <= 0
}

// overflowEpochOf returns the overflow epoch under mtx
func (w *Writer) overflowEpochOf() uint64 {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.overflowEpoch
}

// overflowFrozen reports whether an overflow freezes the cursors of the worker's writes (D21). A resync pass rewrites
// the groups with source cursors, so the overflow does not freeze that pass.
func (w *Writer) overflowFrozen() bool {
	if w.resyncPass {
		return false
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.overflow
}

// clearOverflowIfUnchanged ends the overflow freeze after a resync with a source completed with no skip in any group,
// and no overflow happened since the resync started (D19 rule b). A skip keeps the overflow, so the frozen groups stay frozen.
func (w *Writer) clearOverflowIfUnchanged(epoch uint64) {
	if w.skipsSeen {
		return
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.overflowEpoch == epoch {
		w.overflow = false
	}
}

// readMeta reads the meta hash once per resync. Errors are retried like a failed script call.
func (w *Writer) readMeta() (map[string]string, error) {
	return w.readHash(w.keys.MetaUnprefixed())
}

// readHash reads a whole hash by its unprefixed name. Errors are retried like a failed script call.
func (w *Writer) readHash(key string) (map[string]string, error) {
	var fields map[string]string
	err := w.retry("redis hash read", func() error {
		f, e := w.client.HGetAll(key)
		if e == nil {
			fields = f
			w.recordSuccess()
		}
		return e
	})
	return fields, err
}

// withRemovals appends the entities the source does not list and marks them removed, so the script tombstones them (D40)
func withRemovals[T any](listed, gone []T) ([]T, []bool) {
	all := make([]T, 0, len(listed)+len(gone))
	all = append(all, listed...)
	all = append(all, gone...)
	removals := make([]bool, len(all))
	for i := len(listed); i < len(all); i++ {
		removals[i] = true
	}
	return all, removals
}

// unlistedFlags returns the stored flags the source does not list. Each keeps the change number of its own stored JSON (D4),
// because the source has no entry for it. An unreadable JSON is returned as a skip, so the cursor does not pass it (D40).
func (w *Writer) unlistedFlags(stored map[string]string, listed []dtos.SplitDTO) ([]dtos.SplitDTO, []string) {
	in := make(map[string]bool, len(listed))
	for _, d := range listed {
		in[d.Name] = true
	}
	var out []dtos.SplitDTO
	var skipped []string
	for name, raw := range stored {
		if in[name] {
			continue
		}
		var d dtos.SplitDTO
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			w.logger.Error(fmt.Sprintf("redis state: stored flag %q has unreadable JSON, its removal is not written", name))
			skipped = append(skipped, name)
			continue
		}
		d.Name = name
		out = append(out, d)
	}
	return out, skipped
}

// unlistedRuleBasedSegments is unlistedFlags for rule-based segments
func (w *Writer) unlistedRuleBasedSegments(stored map[string]string, listed []dtos.RuleBasedSegmentDTO) ([]dtos.RuleBasedSegmentDTO, []string) {
	in := make(map[string]bool, len(listed))
	for _, d := range listed {
		in[d.Name] = true
	}
	var out []dtos.RuleBasedSegmentDTO
	var skipped []string
	for name, raw := range stored {
		if in[name] {
			continue
		}
		var d dtos.RuleBasedSegmentDTO
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			w.logger.Error(fmt.Sprintf("redis state: stored rule-based segment %q has unreadable JSON, its removal is not written", name))
			skipped = append(skipped, name)
			continue
		}
		d.Name = name
		out = append(out, d)
	}
	return out, skipped
}

// resyncSegments streams the segments from the source. A segment whose stored cursor is already at or above the
// source cursor is skipped. The first error from writing a segment stops the stream and is returned.
func (w *Writer) resyncSegments(source Source, fields map[string]string) (map[string]struct{}, error) {
	var fatal error
	var listed map[string]struct{}
	err := w.retry("source segments read", func() error {
		listed = map[string]struct{}{}
		e := source.Segments(func(s SegmentState) error {
			if fatal != nil {
				return fatal
			}
			listed[s.Name] = struct{}{}
			fatal = w.resyncSegment(s, fields)
			return fatal
		})
		if fatal != nil {
			return nil
		}
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the segments from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if fatal != nil {
		return nil, fatal
	}
	return listed, err
}

// dropUnlistedSegments deletes the segments the meta hash registers but the source did not list (D40, T163). The
// names come from the meta read at the start of the pass, so no key scan is needed. It runs only after a complete stream.
func (w *Writer) dropUnlistedSegments(fields map[string]string, listed map[string]struct{}) error {
	if len(listed) == 0 {
		// an empty source is not evidence that the segments were removed (T62); Reset clears them on a full boot
		return nil
	}
	dropped := 0
	defer func() {
		if dropped > 0 {
			w.logger.Warning(fmt.Sprintf("redis state: deleted %d segments that are in Redis but not in the local state", dropped))
		}
	}()
	for field := range fields {
		name, ok := segmentNameOfCursorField(field)
		if !ok || !validSegmentName(name) {
			continue
		}
		if _, ok := listed[name]; ok {
			continue
		}
		err := w.retry("segment drop", func() error {
			return w.client.Eval(SegmentDropScript, w.keys.SegmentKeys(name), name)
		})
		if err != nil {
			return fmt.Errorf("deleting segment %q: %w", name, err)
		}
		dropped++
	}
	return nil
}

// resyncSegment writes one segment unless Redis already holds it up to its change number, with a matching key
// count (D15).
func (w *Writer) resyncSegment(s SegmentState, fields map[string]string) error {
	if !validSegmentName(s.Name) {
		w.reject(fmt.Sprintf("redis state: segment name %q contains glob characters or is empty, not resynced", s.Name))
		return nil
	}
	field := w.keys.SegmentCursorField(s.Name)
	released := w.releaseGroup(field)
	frozen := released.whole || len(released.names) > 0
	err := w.syncSegment(s, field, fields, frozen)
	if err != nil {
		w.restoreGroup(field, released)
	}
	return err
}

// syncSegment is the body of resyncSegment. A segment whose group a dropped write froze is rewritten even when Redis
// looks current, because the lost change may have swapped keys without changing the count.
func (w *Writer) syncSegment(s SegmentState, field string, fields map[string]string, frozen bool) error {
	if !frozen && storedCursor(fields, field) >= s.ChangeNumber {
		intact, err := w.segmentIntact(s)
		if err != nil {
			return fmt.Errorf("checking segment %q: %w", s.Name, err)
		}
		if intact {
			return nil
		}
		w.logger.Warning(fmt.Sprintf("redis state: segment %s has a current cursor but a different key count in Redis, rewriting it", s.Name))
		// the rewrite only adds keys, so the keys Redis holds beyond the source are deleted first (D38).
		// Known window: until the rewrite lands, the committed cursor stays with empty sets, so a reader booting
		// right now restores this segment as empty. Rare (count-mismatch repair only) and healed by the next resync
		if err := w.DeleteSegmentKeys(s.Name); err != nil {
			return fmt.Errorf("deleting keys of segment %q: %w", s.Name, err)
		}
	}
	_, err := w.writeSegment(s.Name, toSegmentKeys(s.Active), toSegmentKeys(s.Removed), s.ChangeNumber, false)
	if err != nil {
		return fmt.Errorf("writing segment %q: %w", s.Name, err)
	}
	return nil
}

// storedCursor parses a cursor field of the meta hash; absent or unparsable means missingCursor
func storedCursor(fields map[string]string, field string) int64 {
	v, err := ParseCursor(fields[field])
	if err != nil {
		return missingCursor
	}
	return v
}

// toSegmentKeys converts a key map into a list sorted by key, so the calls are deterministic
func toSegmentKeys(m map[string]int64) []segmentKey {
	out := make([]segmentKey, 0, len(m))
	for k, cn := range m {
		out = append(out, segmentKey{key: k, cn: cn})
	}
	slices.SortFunc(out, func(a, b segmentKey) int { return strings.Compare(a.key, b.key) })
	return out
}

// segmentIntact reports whether Redis registers the segment and holds as many active keys as the local state
func (w *Writer) segmentIntact(s SegmentState) (bool, error) {
	intact := false
	err := w.retry("segment check", func() error {
		e := w.client.Eval(SegmentCheckScript, w.keys.SegmentKeys(s.Name), len(s.Active), s.Name)
		if ErrorHasMarker(e, countMismatchMarker) {
			return nil
		}
		if e == nil {
			intact = true
		}
		return e
	})
	if errors.Is(err, errChunkRejected) {
		return false, nil
	}
	return intact, err
}

// purgeBatch returns the entries removed per PurgeScript call (Options.PurgeBatch, the default when unset)
func (w *Writer) purgeBatch() int64 {
	if w.opts.PurgeBatch <= 0 {
		return defaultPurgeBatch
	}
	return int64(w.opts.PurgeBatch)
}

// purge removes the expired removal markers (D27): the flag and rule-based segment removal indexes, then the tombstone
// index of every segment named in meta (a -1 cursor still names the segment). Each index is purged in PurgeScript calls
// of at most PurgeBatch entries, repeated while a call removed a full batch. It checks stopped() between calls and
// returns errStopped when the writer stops first. A fingerprint mismatch returns ErrEnvironmentMismatch and deletes nothing.
// It does not need a Source.
func (w *Writer) purge(fields map[string]string) error {
	batch := w.purgeBatch()
	retention := w.opts.Retention.Milliseconds()
	type target struct {
		keys    []string
		hasData string
	}
	targets := []target{{w.keys.FlagKeys(), "1"}, {w.keys.RuleBasedSegmentKeys(), "1"}}
	names := []string{}
	for field := range fields {
		if name, ok := segmentNameOfCursorField(field); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		targets = append(targets, target{w.keys.SegmentKeys(name), "0"})
	}
	for _, t := range targets {
		for {
			if w.stopped() {
				return errStopped
			}
			err := w.retry("expiry purge", func() error {
				return w.client.Eval(PurgeScript, slices.Concat(t.keys, []string{w.keys.PurgeLast()}), w.fingerprint, retention, batch, t.hasData)
			})
			if err != nil {
				return err
			}
			removed, err := w.purgeLastCount()
			if err != nil {
				return err
			}
			if removed < batch {
				break
			}
		}
	}
	return nil
}

// startupPurge is the expiry pass at writer start (D27): meta gives the fingerprint check and the segment names, then purge.
// A purge that fails is logged and reruns at the next start (FL-11). It returns errStopped so the worker exits.
func (w *Writer) startupPurge() error {
	fields, err := w.readMeta()
	if err != nil {
		w.logger.Error(fmt.Sprintf("redis state: reading meta for the startup expiry purge failed, it reruns at the next start: %s", err.Error()))
		return nil
	}
	if fp, ok := fields[FingerprintField]; ok && fp != w.fingerprint {
		w.enterMismatch()
		return ErrEnvironmentMismatch
	}
	err = w.purge(fields)
	switch {
	case errors.Is(err, errStopped):
		w.logger.Info("redis state: startup expiry purge cut short by Stop, it reruns at the next start")
		return err
	case err != nil:
		w.logger.Error(fmt.Sprintf("redis state: the startup expiry purge stopped early, it reruns at the next start: %s", err.Error()))
	}
	return nil
}

// purgeLastCount returns the count of the last PurgeScript call. It uses GET, never HGETALL: the meta hash grows with segments (D40).
func (w *Writer) purgeLastCount() (int64, error) {
	var raw string
	err := w.retry("purge count read", func() error {
		var e error
		raw, e = w.client.Get(w.keys.Unprefixed().PurgeLast())
		return e
	})
	if err != nil {
		return 0, err
	}
	n, err := ParseCursor(raw)
	if err != nil {
		return 0, fmt.Errorf("redis state: reading the purge count: %w", err)
	}
	return n, nil
}
