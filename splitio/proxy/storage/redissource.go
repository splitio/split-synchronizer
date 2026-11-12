package storage

import (
	"errors"
	"fmt"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
)

// RedisStateSource exposes the proxy's complete local state to the Redis writer for resync
type RedisStateSource struct {
	splits   *ProxySplitStorageImpl
	rbs      *ProxyRuleBasedSegmentsStorageImpl
	segments *ProxySegmentStorageImpl
}

func NewRedisStateSource(splits *ProxySplitStorageImpl, rbs *ProxyRuleBasedSegmentsStorageImpl, segments *ProxySegmentStorageImpl) *RedisStateSource {
	return &RedisStateSource{splits: splits, rbs: rbs, segments: segments}
}

var _ redisstate.Source = (*RedisStateSource)(nil)

// Flags returns all flags (archived included) and the flag change number; the cursor is never ahead of the returned data
func (s *RedisStateSource) Flags() ([]dtos.SplitDTO, int64, error) {
	// The cursor is read before the data: a concurrent Update can only leave the cursor behind the data read
	cn := s.splits.db.ChangeNumber()
	all, err := s.splits.db.FetchAll()
	if err != nil {
		if errors.Is(err, persistent.ErrorBucketNotFound) {
			if cn > 0 {
				return nil, 0, fmt.Errorf("flags bucket is missing while change number is %d: %w", cn, err)
			}
			return []dtos.SplitDTO{}, cn, nil
		}
		return nil, 0, fmt.Errorf("reading flags: %w", err)
	}
	for idx := range all {
		// Flags outside the configured flag sets are kept locally but must reach Redis as removed, as on the live path
		if all[idx].Status == constants.SplitStatusActive && !s.splits.flagSets.Instersect(all[idx].Sets) {
			all[idx].Status = constants.SplitStatusArchived
		}
	}
	return all, cn, nil
}

// RuleBasedSegments returns all rule-based segments and their change number; the cursor is never ahead of the returned data
func (s *RedisStateSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	// The cursor is read before the data, as in Flags
	cn := s.rbs.db.ChangeNumber()
	all, err := s.rbs.db.FetchAll()
	if err != nil {
		if errors.Is(err, persistent.ErrorBucketNotFound) {
			if cn > 0 {
				return nil, 0, fmt.Errorf("rule-based segments bucket is missing while change number is %d: %w", cn, err)
			}
			return []dtos.RuleBasedSegmentDTO{}, cn, nil
		}
		return nil, 0, fmt.Errorf("reading rule-based segments: %w", err)
	}
	return all, cn, nil
}

// Segments visits one segment at a time. The change number of a segment is read before its keys,
// so the cursor is never ahead of the data (cursor behind data is safe)
func (s *RedisStateSource) Segments(visit func(redisstate.SegmentState) error) error {
	impl, ok := s.segments.db.(*persistent.SegmentChangesCollectionImpl)
	if !ok {
		return fmt.Errorf("segment collection %T does not support listing", s.segments.db)
	}

	for _, name := range impl.Names() {
		cn := impl.ChangeNumber(name)
		item, err := impl.Fetch(name)
		if err != nil {
			if errors.Is(err, persistent.ErrorKeyNotFound) {
				continue
			}
			return fmt.Errorf("fetching segment %s: %w", name, err)
		}
		if item == nil {
			continue
		}

		state := redisstate.SegmentState{
			Name:         name,
			Active:       make(map[string]int64),
			Removed:      make(map[string]int64),
			ChangeNumber: cn,
		}
		for key, k := range item.Keys {
			if k.Removed {
				state.Removed[key] = k.ChangeNumber
			} else {
				state.Active[key] = k.ChangeNumber
			}
		}
		if err := visit(state); err != nil {
			return err
		}
	}
	return nil
}
