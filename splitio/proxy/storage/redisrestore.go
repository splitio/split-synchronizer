package storage

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
)

// errDuplicateEntityName and errSegmentKeyConflict mark restore input the storages must not accept.
var (
	errDuplicateEntityName = errors.New("duplicate entity name in restore input")
	errSegmentKeyConflict  = errors.New("segment has a key both active and removed")
)

// errSegmentCollectionNotRestorable is returned when the segment collection does not implement Restore.
var errSegmentCollectionNotRestorable = errors.New("segment collection does not support restore")

// ApplyCommittedCursors sets the committed change numbers from a BootValid state. A cursor below the max restored cn
// is kept, so the next since-request returns newer items again. Negative cursors and state.SkippedSegments are skipped.
func ApplyCommittedCursors(
	state redisstate.BootState,
	splits *ProxySplitStorageImpl,
	rbs *ProxyRuleBasedSegmentsStorageImpl,
	segs *ProxySegmentStorageImpl,
) error {
	if state.Status != redisstate.BootValid {
		return nil
	}

	if state.FlagCursor >= 0 {
		if err := splits.SetChangeNumber(state.FlagCursor); err != nil {
			return fmt.Errorf("error applying flags cursor %d: %w", state.FlagCursor, err)
		}
		anchorFlagsStartingPoint(splits, state.FlagCursor)
	}

	if state.RuleBasedSegmentCursor >= 0 {
		if err := rbs.SetChangeNumber(state.RuleBasedSegmentCursor); err != nil {
			return fmt.Errorf("error applying rule-based segments cursor %d: %w", state.RuleBasedSegmentCursor, err)
		}
		anchorRuleBasedSegmentsStartingPoint(rbs, state.RuleBasedSegmentCursor)
	}

	skipped := skippedSet(state.SkippedSegments)
	for name, cn := range state.SegmentCursors {
		if cn < 0 || skipped.Has(name) {
			continue
		}
		if err := segs.SetChangeNumber(name, cn); err != nil {
			return fmt.Errorf("error applying cursor for segment %s: %w", name, err)
		}
	}
	return nil
}

// anchorFlagsStartingPoint makes cn the oldest change number answered incrementally. History below the cursor is
// incomplete after a restore (the writer purges removal tombstones), so keeping the lowest restored cn would let a
// stale SDK miss a purged removal. It assigns rather than keeps the minimum; later changes are redelivered by sync.
// Callers skip negative cursors.
func anchorFlagsStartingPoint(p *ProxySplitStorageImpl, cn int64) {
	p.mtx.Lock()
	p.oldestKnownCN = cn
	p.mtx.Unlock()
}

// anchorRuleBasedSegmentsStartingPoint is anchorFlagsStartingPoint for rule-based segments.
func anchorRuleBasedSegmentsStartingPoint(p *ProxyRuleBasedSegmentsStorageImpl, cn int64) {
	p.mtx.Lock()
	p.oldestKnownCN = cn
	p.mtx.Unlock()
}

// restoreFlags applies each change number group to the snapshot and Bolt in ascending order. The historic view is
// updated once after the loop with all groups accumulated, because HistoricChangesImpl.Update ignores its cn and
// re-sorts the whole view on every call. This matches per-cn updates only when each name appears at most once in
// flags, which the redis reader guarantees (one entry per name, see redisstate reader readEntities).
func restoreFlags(p *ProxySplitStorageImpl, flags []dtos.SplitDTO) error {
	if err := rejectDuplicateNames(flags, func(s dtos.SplitDTO) string { return s.Name }); err != nil {
		return err
	}
	var allAdd, allRemove []dtos.SplitDTO
	var maxCN int64
	err := forEachChangeNumber(flags, func(s dtos.SplitDTO) int64 { return s.ChangeNumber }, func(group []dtos.SplitDTO, cn int64) error {
		var toAdd, toRemove []dtos.SplitDTO
		for _, s := range group {
			if s.Status == constants.SplitStatusActive {
				toAdd = append(toAdd, s)
			} else {
				toRemove = append(toRemove, s)
			}
		}
		p.setStartingPoint(cn)
		p.mtx.Lock()
		p.snapshot.Update(toAdd, toRemove, cn)
		err := p.db.UpdateWithError(toAdd, toRemove, cn)
		p.mtx.Unlock()
		if err != nil {
			return fmt.Errorf("persisting flags at cn %d: %w", cn, err)
		}
		allAdd = append(allAdd, toAdd...)
		allRemove = append(allRemove, toRemove...)
		maxCN = cn
		return nil
	})
	if err != nil {
		return err
	}
	if len(flags) > 0 {
		p.mtx.Lock()
		p.historic.Update(allAdd, allRemove, maxCN)
		p.mtx.Unlock()
	}
	return nil
}

// restoreRuleBasedSegments is restoreFlags for rule-based segments.
func restoreRuleBasedSegments(p *ProxyRuleBasedSegmentsStorageImpl, rbs []dtos.RuleBasedSegmentDTO) error {
	if err := rejectDuplicateNames(rbs, func(rb dtos.RuleBasedSegmentDTO) string { return rb.Name }); err != nil {
		return err
	}
	var allAdd, allRemove []dtos.RuleBasedSegmentDTO
	var maxCN int64
	err := forEachChangeNumber(rbs, func(rb dtos.RuleBasedSegmentDTO) int64 { return rb.ChangeNumber }, func(group []dtos.RuleBasedSegmentDTO, cn int64) error {
		var toAdd, toRemove []dtos.RuleBasedSegmentDTO
		for _, rb := range group {
			if rb.Status == constants.SplitStatusActive {
				toAdd = append(toAdd, rb)
			} else {
				toRemove = append(toRemove, rb)
			}
		}
		p.setStartingPoint(cn)
		p.mtx.Lock()
		if err := p.snapshot.Update(toAdd, toRemove, cn); err != nil {
			p.mtx.Unlock()
			return fmt.Errorf("restoring rule-based segments snapshot at cn %d: %w", cn, err)
		}
		err := p.db.UpdateWithError(toAdd, toRemove, cn)
		p.mtx.Unlock()
		if err != nil {
			return fmt.Errorf("persisting rule-based segments at cn %d: %w", cn, err)
		}
		allAdd = append(allAdd, toAdd...)
		allRemove = append(allRemove, toRemove...)
		maxCN = cn
		return nil
	})
	if err != nil {
		return err
	}
	if len(rbs) > 0 {
		p.mtx.Lock()
		p.historic.Update(allAdd, allRemove, maxCN)
		p.mtx.Unlock()
	}
	return nil
}

// rejectDuplicateNames errors when two items share a name: the batched historic update is only equivalent to per
// change number updates when each name appears once.
func rejectDuplicateNames[T any](items []T, nameOf func(T) string) error {
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		n := nameOf(it)
		if _, dup := seen[n]; dup {
			return fmt.Errorf("%w: %q", errDuplicateEntityName, n)
		}
		seen[n] = struct{}{}
	}
	return nil
}

// restoreSegment restores one segment into Bolt in a single transaction and updates its caches.
func restoreSegment(s *ProxySegmentStorageImpl, st redisstate.SegmentState) error {
	collection, ok := s.db.(*persistent.SegmentChangesCollectionImpl)
	if !ok {
		return errSegmentCollectionNotRestorable
	}
	item := persistent.SegmentChangesItem{
		Name: st.Name,
		Keys: make(map[string]persistent.SegmentKey, len(st.Active)+len(st.Removed)),
	}
	added := set.NewSet()
	for key, cn := range st.Active {
		item.Keys[key] = persistent.SegmentKey{Name: key, ChangeNumber: cn}
		added.Add(key)
	}
	removed := set.NewSet()
	for key, cn := range st.Removed {
		if _, both := st.Active[key]; both {
			return fmt.Errorf("%w: %s", errSegmentKeyConflict, st.Name)
		}
		item.Keys[key] = persistent.SegmentKey{Name: key, ChangeNumber: cn, Removed: true}
		removed.Add(key)
	}

	if err := collection.Restore(item); err != nil {
		return fmt.Errorf("segment %s: %w", st.Name, err)
	}
	if err := s.mysegments.Update(st.Name, added, removed); err != nil {
		return fmt.Errorf("error restoring segment %s membership cache: %w", st.Name, err)
	}
	s.nameCountCache.Update(st.Name, added.Size(), 0) // tombstones are not part of the key count
	return nil
}

// RedisRestoreSink restores a redis state into the storages as the reader streams it: the flags and rule-based
// segments first, then one segment at a time, so only one segment payload is alive at once. It never
// writes to redis. Call ApplyCommittedCursors after a BootValid read.
// A failed or non-valid read leaves partial data behind: discard the storages and their DB.
// Build it with NewRedisRestoreSink (the zero value is not usable) and use it from one goroutine only.
type RedisRestoreSink struct {
	splits *ProxySplitStorageImpl
	rbs    *ProxyRuleBasedSegmentsStorageImpl
	segs   *ProxySegmentStorageImpl
	seen   map[string]struct{} // segment names already restored, to reject a repeated name
}

var _ redisstate.RestoreSink = (*RedisRestoreSink)(nil)

// NewRedisRestoreSink returns a sink that restores into the given storages.
func NewRedisRestoreSink(splits *ProxySplitStorageImpl, rbs *ProxyRuleBasedSegmentsStorageImpl, segs *ProxySegmentStorageImpl) *RedisRestoreSink {
	return &RedisRestoreSink{splits: splits, rbs: rbs, segs: segs, seen: map[string]struct{}{}}
}

// Entities restores the flags, then the rule-based segments.
func (k *RedisRestoreSink) Entities(flags []dtos.SplitDTO, ruleBasedSegments []dtos.RuleBasedSegmentDTO) error {
	if err := restoreFlags(k.splits, flags); err != nil {
		return fmt.Errorf("error restoring flags: %w", err)
	}
	if err := restoreRuleBasedSegments(k.rbs, ruleBasedSegments); err != nil {
		return fmt.Errorf("error restoring rule-based segments: %w", err)
	}
	return nil
}

// Segment restores one segment.
func (k *RedisRestoreSink) Segment(st redisstate.SegmentState) error {
	if _, dup := k.seen[st.Name]; dup {
		return fmt.Errorf("%w: %q", errDuplicateEntityName, st.Name)
	}
	k.seen[st.Name] = struct{}{}
	return restoreSegment(k.segs, st)
}

// forEachChangeNumber calls apply once per distinct change number in ascending order, and stops at the first error.
func forEachChangeNumber[T any](items []T, cnOf func(T) int64, apply func(group []T, cn int64) error) error {
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b T) int { return cmp.Compare(cnOf(a), cnOf(b)) })
	for start := 0; start < len(sorted); {
		cn := cnOf(sorted[start])
		end := start
		for end < len(sorted) && cnOf(sorted[end]) == cn {
			end++
		}
		if err := apply(sorted[start:end], cn); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func skippedSet(names []string) *set.ThreadUnsafeSet {
	out := set.NewSet()
	for _, name := range names {
		out.Add(name)
	}
	return out
}
