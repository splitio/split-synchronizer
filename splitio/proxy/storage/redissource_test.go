package storage

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bolt "go.etcd.io/bbolt"
)

func TestRedisStateSource(t *testing.T) {
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	require.Nil(t, err)

	splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	segments := NewProxySegmentStorage(dbw, logger, false)

	splits.Update([]dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"}},
		[]dtos.SplitDTO{{Name: "f2", ChangeNumber: 4, Status: "ARCHIVED", TrafficTypeName: "ttt"}}, 4)
	assert.Nil(t, rbs.Update([]dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5))
	assert.Nil(t, segments.Update("seg", set.NewSet("a", "b"), set.NewSet("c"), 7))

	source := NewRedisStateSource(splits, rbs, segments)
	var _ redisstate.Source = source

	flags, cn, err := source.Flags()
	require.Nil(t, err)
	assert.Equal(t, int64(4), cn)
	names := make([]string, 0, len(flags))
	for _, f := range flags {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(t, []string{"f1", "f2"}, names)

	rbDTOs, rbCN, err := source.RuleBasedSegments()
	require.Nil(t, err)
	assert.Equal(t, int64(5), rbCN)
	require.Len(t, rbDTOs, 1)
	assert.Equal(t, "rb1", rbDTOs[0].Name)

	var states []redisstate.SegmentState
	err = source.Segments(func(s redisstate.SegmentState) error {
		states = append(states, s)
		return nil
	})
	require.Nil(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "seg", states[0].Name)
	assert.Equal(t, int64(7), states[0].ChangeNumber)
	assert.Equal(t, map[string]int64{"a": 7, "b": 7}, states[0].Active)
	assert.Equal(t, map[string]int64{"c": 7}, states[0].Removed)
}

func TestRedisStateSourceFlagsCursorMatchesData(t *testing.T) {
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	require.Nil(t, err)

	splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	segments := NewProxySegmentStorage(dbw, logger, false)
	source := NewRedisStateSource(splits, rbs, segments)

	const updates = 300
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(1); i <= updates; i++ {
			splits.Update([]dtos.SplitDTO{{Name: "f", ChangeNumber: i, Status: "ACTIVE", TrafficTypeName: "ttt"}}, nil, i)
		}
	}()

	for i := 0; i < 200; i++ {
		flags, cn, err := source.Flags()
		require.Nil(t, err)
		var maxCN int64
		for _, f := range flags {
			if f.ChangeNumber > maxCN {
				maxCN = f.ChangeNumber
			}
		}
		if len(flags) > 0 {
			assert.LessOrEqual(t, cn, maxCN, "cursor must never be ahead of the returned flags")
		}
	}
	wg.Wait()

	flags, cn, err := source.Flags()
	require.Nil(t, err)
	require.Len(t, flags, 1)
	assert.Equal(t, int64(updates), cn)
	assert.Equal(t, int64(updates), flags[0].ChangeNumber)
}

func TestRedisStateSourceSkipsMissingSegmentItem(t *testing.T) {
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	require.Nil(t, err)

	splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	segments := NewProxySegmentStorage(dbw, logger, false)

	assert.Nil(t, segments.Update("seg1", set.NewSet("a"), set.NewSet(), 7))
	assert.Nil(t, segments.Update("seg2", set.NewSet("b"), set.NewSet(), 8))
	segImpl := segments.db.(*persistent.SegmentChangesCollectionImpl)
	segImpl.SetChangeNumber("ghost", 9)

	source := NewRedisStateSource(splits, rbs, segments)

	var states []redisstate.SegmentState
	err = source.Segments(func(s redisstate.SegmentState) error {
		states = append(states, s)
		return nil
	})
	require.Nil(t, err)

	names := make([]string, 0, len(states))
	for _, st := range states {
		names = append(names, st.Name)
	}
	assert.ElementsMatch(t, []string{"seg1", "seg2"}, names)
	assert.NotContains(t, names, "ghost")
}

func TestRedisStateSourceEmptyBucketFreshInstall(t *testing.T) {
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	require.Nil(t, err)

	splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	segments := NewProxySegmentStorage(dbw, logger, false)
	source := NewRedisStateSource(splits, rbs, segments)

	flags, cn, err := source.Flags()
	require.Nil(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, int64(-1), cn)

	rbDTOs, rbCN, err := source.RuleBasedSegments()
	require.Nil(t, err)
	assert.Empty(t, rbDTOs)
	assert.Equal(t, int64(-1), rbCN)
}

func TestRedisStateSourceMissingBucketWithCursorIsError(t *testing.T) {
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	require.Nil(t, err)

	splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	segments := NewProxySegmentStorage(dbw, logger, false)

	splits.Update([]dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"}}, nil, 4)
	assert.Nil(t, rbs.Update([]dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5))

	// Simulate a lost Bolt bucket: the in-memory snapshot keeps cn != 0 but the buckets are gone
	require.Nil(t, dbw.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("SPLIT_CHANGES_COLLECTION")); err != nil {
			return err
		}
		return tx.DeleteBucket([]byte("RULE_BASED_SEGMENTS_CHANGES_COLLECTION"))
	}))

	source := NewRedisStateSource(splits, rbs, segments)

	_, _, err = source.Flags()
	require.NotNil(t, err)
	assert.ErrorIs(t, err, persistent.ErrorBucketNotFound)
	assert.Contains(t, err.Error(), "bucket is missing while change number is 4")

	_, _, err = source.RuleBasedSegments()
	require.NotNil(t, err)
	assert.ErrorIs(t, err, persistent.ErrorBucketNotFound)
	assert.Contains(t, err.Error(), "bucket is missing while change number is 5")
}
