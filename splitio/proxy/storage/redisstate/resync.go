package redisstate

import (
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
		return err
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
		return err
	}
	ents, skipped := w.flagEntities(flags, nil)
	released := w.releaseGroup(w.keys.FlagCursorField())
	if _, err := w.writeEntities("flag resync", FlagScript, w.keys.FlagKeys(), w.keys.FlagCursorField(), ents, flagsCn, skipped); err != nil {
		w.restoreGroup(w.keys.FlagCursorField(), released)
		return err
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
		return err
	}
	rbsEnts, rbsSkipped := w.rbsEntities(rbs, nil)
	releasedRBS := w.releaseGroup(w.keys.RuleBasedSegmentCursorField())
	if _, err := w.writeEntities("rule-based segment resync", RuleBasedSegmentScript, w.keys.RuleBasedSegmentKeys(), w.keys.RuleBasedSegmentCursorField(), rbsEnts, rbsCn, rbsSkipped); err != nil {
		w.restoreGroup(w.keys.RuleBasedSegmentCursorField(), releasedRBS)
		return err
	}

	if err := w.resyncSegments(source, fields); err != nil {
		return err
	}
	w.clearOverflowIfUnchanged(epoch)
	return nil
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
	var fields map[string]string
	err := w.retry("redis meta read", func() error {
		f, e := w.client.HGetAll(w.keys.MetaUnprefixed())
		if e == nil {
			fields = f
			w.recordSuccess()
		}
		return e
	})
	return fields, err
}

// resyncSegments streams the segments from the source. A segment whose stored cursor is already at or above the
// source cursor is skipped. The first error from writing a segment stops the stream and is returned.
func (w *Writer) resyncSegments(source Source, fields map[string]string) error {
	var fatal error
	err := w.retry("source segments read", func() error {
		e := source.Segments(func(s SegmentState) error {
			if fatal != nil {
				return fatal
			}
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
		return fatal
	}
	return err
}

// resyncSegment writes one segment unless Redis already holds it up to its change number, with a matching key
// count (D15).
func (w *Writer) resyncSegment(s SegmentState, fields map[string]string) error {
	if !validSegmentName(s.Name) {
		w.reject(fmt.Sprintf("redis state: segment name %q contains glob characters or is empty, not resynced", s.Name))
		return nil
	}
	field := w.keys.SegmentCursorField(s.Name)
	if storedCursor(fields, field) >= s.ChangeNumber {
		intact, err := w.segmentIntact(s)
		if err != nil || intact {
			return err
		}
		w.logger.Warning(fmt.Sprintf("redis state: segment %s has a current cursor but a different key count in Redis, rewriting it", s.Name))
	}
	_, err := w.writeSegment(s.Name, toSegmentKeys(s.Active), toSegmentKeys(s.Removed), s.ChangeNumber, false)
	return err
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
				return w.client.Eval(PurgeScript, t.keys, w.fingerprint, retention, batch, t.hasData)
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

// purgeLastCount reads the number of entries the last PurgeScript call removed (the purgeLast meta field)
func (w *Writer) purgeLastCount() (int64, error) {
	fields, err := w.readMeta()
	if err != nil {
		return 0, err
	}
	n, err := ParseCursor(fields[purgeLastField])
	if err != nil {
		return 0, fmt.Errorf("redis state: reading the purge count: %w", err)
	}
	return n, nil
}
