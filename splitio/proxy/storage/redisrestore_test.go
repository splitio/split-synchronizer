package storage

import (
	"errors"
	"path/filepath"
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

	"context"
	"fmt"
	goredis "github.com/redis/go-redis/v9"
	config "github.com/splitio/go-split-commons/v10/conf"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// restoreFromRedisState applies a collected BootState through the sink the same way ReadInto does: flags and
// rule-based segments first, then each segment not listed in state.SkippedSegments. It is the bulk reference the
// sink tests compare against. It returns nil without touching the storages unless state.Status is BootValid.
func restoreFromRedisState(
	state redisstate.BootState,
	splits *ProxySplitStorageImpl,
	rbs *ProxyRuleBasedSegmentsStorageImpl,
	segs *ProxySegmentStorageImpl,
) error {
	if state.Status != redisstate.BootValid {
		return nil
	}

	sink := NewRedisRestoreSink(splits, rbs, segs)
	if err := sink.Entities(state.Flags, state.RuleBasedSegments); err != nil {
		return err
	}
	skipped := skippedSet(state.SkippedSegments)
	for _, st := range state.Segments {
		if skipped.Has(st.Name) {
			continue
		}
		if err := sink.Segment(st); err != nil {
			return err
		}
	}
	return nil
}

// redisWriteCounter counts redis writer calls. Zero means restore did not go through Update.
type redisWriteCounter struct {
	n int
}

func (c *redisWriteCounter) PutFlags(_, _ []dtos.SplitDTO, _ int64) { c.n++ }

func (c *redisWriteCounter) PutRuleBasedSegments(_, _ []dtos.RuleBasedSegmentDTO, _ int64) { c.n++ }

func (c *redisWriteCounter) PutSegment(_ string, _, _ []string, _ int64) { c.n++ }

func (c *redisWriteCounter) PutSegmentFull(_ string, _ []string, _ int64) { c.n++ }

type restoreStores struct {
	db     persistent.DBWrapper
	splits *ProxySplitStorageImpl
	rbs    *ProxyRuleBasedSegmentsStorageImpl
	segs   *ProxySegmentStorageImpl
	writes *redisWriteCounter
}

func newRestoreStores(t *testing.T) restoreStores {
	t.Helper()
	logger := logging.NewLogger(nil)
	dbw, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	require.Nil(t, err)
	t.Cleanup(func() { _ = dbw.Discard() })

	st := restoreStores{
		db:     dbw,
		splits: NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false),
		rbs:    NewProxyRuleBasedSegmentsStorage(dbw, logger, false),
		segs:   NewProxySegmentStorage(dbw, logger, false),
		writes: &redisWriteCounter{},
	}
	st.splits.WithRedisWriter(st.writes)
	st.rbs.WithRedisWriter(st.writes)
	st.segs.WithRedisWriter(st.writes)
	return st
}

// failingBolt is a persistent.DBWrapper whose writes always fail.
type failingBolt struct{}

var errDiskFull = errors.New("disk full")

func (failingBolt) Update(func(*bolt.Tx) error) error { return errDiskFull }
func (failingBolt) View(func(*bolt.Tx) error) error   { return nil }
func (failingBolt) Lock()                             {}
func (failingBolt) Unlock()                           {}
func (failingBolt) GetRawSnapshot() ([]byte, error)   { return nil, nil }

func splitNames(dtoList []dtos.SplitDTO) []string {
	names := make([]string, 0, len(dtoList))
	for _, s := range dtoList {
		names = append(names, s.Name)
	}
	return names
}

// BenchmarkRestoreFlagsUniqueCN restores 30000 flags, each with its own change number, into a fresh in-memory Bolt.
func BenchmarkRestoreFlagsUniqueCN(b *testing.B) {
	logger := logging.NewLogger(nil)
	flags := make([]dtos.SplitDTO, 30000)
	for i := range flags {
		flags[i] = dtos.SplitDTO{
			Name:            fmt.Sprintf("flag_%d", i),
			ChangeNumber:    int64(i + 1),
			Status:          "ACTIVE",
			TrafficTypeName: "ttt",
		}
	}
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = dbw.Discard() })
		splits := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), false)
		sink := NewRedisRestoreSink(
			splits,
			NewProxyRuleBasedSegmentsStorage(dbw, logger, false),
			NewProxySegmentStorage(dbw, logger, false),
		)
		b.StartTimer()
		if err := sink.Entities(flags, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// The historic view gets one update after all change-number groups. It must match per-cn updates: views sorted by
// LastUpdated, with the removed flag (cn 4, ARCHIVED) marked inactive. Expected values are computed from the DTOs.
func TestRestoreFlagsHistoricMatchesPerCN(t *testing.T) {
	st := newRestoreStores(t)
	flags := []dtos.SplitDTO{
		{Name: "d", ChangeNumber: 5, Status: "ACTIVE", TrafficTypeName: "ttt"},
		{Name: "c", ChangeNumber: 4, Status: "ARCHIVED", TrafficTypeName: "ttt"},
		{Name: "b", ChangeNumber: 2, Status: "ACTIVE", TrafficTypeName: "ttt"},
		{Name: "a", ChangeNumber: 1, Status: "ACTIVE", TrafficTypeName: "ttt"},
	}
	require.Nil(t, NewRedisRestoreSink(st.splits, st.rbs, st.segs).Entities(flags, nil))

	views := func(since int64) []string {
		out := []string{}
		for _, v := range st.splits.historic.GetUpdatedSince(since, nil) {
			out = append(out, fmt.Sprintf("%s:%t:%d", v.Name, v.Active, v.LastUpdated))
		}
		return out
	}
	assert.Equal(t, []string{"a:true:1", "b:true:2", "c:false:4", "d:true:5"}, views(0))
	assert.Equal(t, []string{"c:false:4", "d:true:5"}, views(2))
	assert.Empty(t, views(5))
}

// TestRestoreIntoBoltThenServe restores into Bolt, then builds new storages from the same Bolt file
// and serves from them.
func TestRestoreIntoBoltThenServe(t *testing.T) {
	st := newRestoreStores(t)

	state := redisstate.BootState{
		Status: redisstate.BootValid,
		Flags: []dtos.SplitDTO{
			{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"},
			{Name: "f2", ChangeNumber: 4, Status: "ARCHIVED", TrafficTypeName: "ttt"},
		},
		RuleBasedSegments:      []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}},
		FlagCursor:             4,
		RuleBasedSegmentCursor: 5,
		SegmentCursors:         map[string]int64{"seg": 7},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 6, "b": 7},
			Removed:      map[string]int64{"c": 5},
			ChangeNumber: 7,
		}},
	}

	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	// Restore must not write to redis or go through ProxySplitStorageImpl.Update.
	assert.Equal(t, 0, st.writes.n)

	// Fresh storages built from the restored Bolt file serve the same data.
	logger := logging.NewLogger(nil)
	splits := NewProxySplitStorage(st.db, logger, flagsets.NewFlagSetFilter(nil), true)
	rbs := NewProxyRuleBasedSegmentsStorage(st.db, logger, true)
	segs := NewProxySegmentStorage(st.db, logger, true)
	require.Nil(t, ApplyCommittedCursors(state, splits, rbs, segs))

	flagsView, err := splits.ChangesSince(-1, nil)
	require.Nil(t, err)
	assert.Equal(t, int64(4), flagsView.Till)
	assert.Equal(t, []string{"f1"}, splitNames(flagsView.Splits))

	_, err = splits.ChangesSince(3, nil)
	assert.ErrorIs(t, err, ErrSinceParamTooOld, "since below the committed flags cursor 4 is too old")

	atCursor, err := splits.ChangesSince(4, nil)
	require.Nil(t, err)
	assert.Equal(t, int64(4), atCursor.Till)
	assert.Empty(t, atCursor.Splits)

	rbView, err := rbs.ChangesSince(-1)
	require.Nil(t, err)
	assert.Equal(t, int64(5), rbView.Till)
	require.Len(t, rbView.RuleBasedSegments, 1)
	assert.Equal(t, "rb1", rbView.RuleBasedSegments[0].Name)

	segAll, err := segs.ChangesSince("seg", -1)
	require.Nil(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, segAll.Added)
	assert.Equal(t, int64(7), segAll.Till)

	// Tombstone for "c" (cn 5) is kept and returned for since 4.
	segSince, err := segs.ChangesSince("seg", 4)
	require.Nil(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, segSince.Added)
	assert.Equal(t, []string{"c"}, segSince.Removed)
	assert.Equal(t, int64(7), segSince.Till)

	assert.Equal(t, 0, st.writes.n)
}

// TestRestoreAppliesCommittedCursors checks that committed cursors below the max item cn are kept;
// a skipped segment gets no cursor.
func TestRestoreAppliesCommittedCursors(t *testing.T) {
	st := newRestoreStores(t)

	state := redisstate.BootState{
		Status: redisstate.BootValid,
		Flags: []dtos.SplitDTO{
			{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"},
			{Name: "f2", ChangeNumber: 9, Status: "ACTIVE", TrafficTypeName: "ttt"},
		},
		RuleBasedSegments:      []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 8, Status: "ACTIVE"}},
		FlagCursor:             5,
		RuleBasedSegmentCursor: 2,
		SegmentCursors:         map[string]int64{"seg": 6, "skipped": 3},
		SkippedSegments:        []string{"skipped"},
		Segments: []redisstate.SegmentState{
			{Name: "seg", Active: map[string]int64{"a": 4, "b": 9}, ChangeNumber: 9},
		},
	}

	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))
	assert.Equal(t, 0, st.writes.n)

	flagCN, err := st.splits.ChangeNumber()
	require.Nil(t, err)
	assert.Equal(t, int64(5), flagCN, "cursor below max item cn must be kept")

	rbCN, err := st.rbs.ChangeNumber()
	require.Nil(t, err)
	assert.Equal(t, int64(2), rbCN, "cursor below max item cn must be kept")

	segCN, err := st.segs.ChangeNumber("seg")
	require.Nil(t, err)
	assert.Equal(t, int64(6), segCN, "cursor below max key cn must be kept")

	skippedCN, err := st.segs.ChangeNumber("skipped")
	require.NoError(t, err)
	assert.NotEqual(t, int64(3), skippedCN, "skipped segment must not get the committed cursor")

	// The first sync asks since the committed cursor and gets the items above it.
	flags, err := st.splits.ChangesSince(5, nil)
	require.Nil(t, err)
	assert.Equal(t, int64(9), flags.Till)
	assert.Equal(t, []string{"f2"}, splitNames(flags.Splits))

	rbs, err := st.rbs.ChangesSince(2)
	require.Nil(t, err)
	assert.Equal(t, int64(8), rbs.Till)
	require.Len(t, rbs.RuleBasedSegments, 1)
	assert.Equal(t, "rb1", rbs.RuleBasedSegments[0].Name)

	segs, err := st.segs.ChangesSince("seg", 6)
	require.Nil(t, err)
	assert.Equal(t, int64(9), segs.Till)
	assert.Equal(t, []string{"b"}, segs.Added)

	assert.Equal(t, 0, st.writes.n)
}

// TestSinceBelowCursorIsTooOld checks that since below the committed cursor is too old; since at
// the cursor and -1 work.
func TestSinceBelowCursorIsTooOld(t *testing.T) {
	st := newRestoreStores(t)

	state := redisstate.BootState{
		Status: redisstate.BootValid,
		Flags: []dtos.SplitDTO{
			{Name: "old_untouched", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "ttt"},
			{Name: "recent", ChangeNumber: 40, Status: "ACTIVE", TrafficTypeName: "ttt"},
		},
		RuleBasedSegments: []dtos.RuleBasedSegmentDTO{
			{Name: "rb_old", ChangeNumber: 10, Status: "ACTIVE"},
			{Name: "rb_recent", ChangeNumber: 40, Status: "ACTIVE"},
		},
		FlagCursor:             40,
		RuleBasedSegmentCursor: 40,
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	_, err := st.splits.ChangesSince(25, nil)
	assert.ErrorIs(t, err, ErrSinceParamTooOld, "flags since below the cursor must be answered as too old")
	_, err = st.rbs.ChangesSince(25)
	assert.ErrorIs(t, err, ErrSinceParamTooOld, "rule-based segments since below the cursor must be answered as too old")

	at, err := st.splits.ChangesSince(40, nil)
	require.Nil(t, err)
	assert.Equal(t, int64(40), at.Till)
	assert.Empty(t, at.Splits)
	all, err := st.splits.ChangesSince(-1, nil)
	require.Nil(t, err)
	assert.ElementsMatch(t, []string{"old_untouched", "recent"}, splitNames(all.Splits))

	rbAt, err := st.rbs.ChangesSince(40)
	require.Nil(t, err)
	assert.Empty(t, rbAt.RuleBasedSegments)
	rbAll, err := st.rbs.ChangesSince(-1)
	require.Nil(t, err)
	assert.Len(t, rbAll.RuleBasedSegments, 2)
}

// TestEmptyRestoreAnchorsAtCursor checks that a restore with no items still anchors the starting
// point at the cursor.
func TestEmptyRestoreAnchorsAtCursor(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{Status: redisstate.BootValid, FlagCursor: 40, RuleBasedSegmentCursor: 40}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	_, err := st.splits.ChangesSince(25, nil)
	assert.ErrorIs(t, err, ErrSinceParamTooOld)
	_, err = st.rbs.ChangesSince(25)
	assert.ErrorIs(t, err, ErrSinceParamTooOld)
}

// TestRestoreNameCountIgnoresTombstones checks that a segment with 1 active key and 3 tombstones
// is counted as 1.
func TestRestoreNameCountIgnoresTombstones(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{
		Status:         redisstate.BootValid,
		SegmentCursors: map[string]int64{"seg": 9},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 8},
			Removed:      map[string]int64{"x": 5, "y": 6, "z": 7},
			ChangeNumber: 9,
		}},
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	counts := st.segs.NamesAndCount()
	require.Contains(t, counts, "seg", "a segment with active keys must be listed even when it has more tombstones")
	assert.Equal(t, 1, counts["seg"])
}

// TestFlagsWriteFailureReturned checks that a failed Bolt write is returned for flags, rule-based
// segments and segments.
func TestFlagsWriteFailureReturned(t *testing.T) {
	logger := logging.NewLogger(nil)
	db := failingBolt{}
	splits := NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segs := NewProxySegmentStorage(db, logger, false)
	state := redisstate.BootState{
		Status:     redisstate.BootValid,
		Flags:      []dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE"}},
		FlagCursor: 3,
	}

	err := restoreFromRedisState(state, splits, rbs, segs)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDiskFull)
}

func TestRuleBasedSegmentsWriteFailureReturned(t *testing.T) {
	logger := logging.NewLogger(nil)
	db := failingBolt{}
	splits := NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segs := NewProxySegmentStorage(db, logger, false)
	state := redisstate.BootState{
		Status:                 redisstate.BootValid,
		RuleBasedSegments:      []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 3, Status: "ACTIVE"}},
		RuleBasedSegmentCursor: 3,
	}

	err := restoreFromRedisState(state, splits, rbs, segs)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDiskFull)
}

func TestSegmentWriteFailureReturned(t *testing.T) {
	logger := logging.NewLogger(nil)
	db := failingBolt{}
	splits := NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segs := NewProxySegmentStorage(db, logger, false)
	state := redisstate.BootState{
		Status:         redisstate.BootValid,
		SegmentCursors: map[string]int64{"seg": 4},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 4},
			ChangeNumber: 4,
		}},
	}

	err := restoreFromRedisState(state, splits, rbs, segs)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDiskFull)
}

// TestRestoredSegmentServesFullSetAfterDelta checks that a restored segment serves all active keys
// for since -1, and still after one delta sync.
func TestRestoredSegmentServesFullSetAfterDelta(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{
		Status:         redisstate.BootValid,
		SegmentCursors: map[string]int64{"seg": 9},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 4, "b": 9},
			Removed:      map[string]int64{"c": 5},
			ChangeNumber: 9,
		}},
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	all, err := st.segs.ChangesSince("seg", -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, all.Added)

	toAdd := set.NewSet()
	toAdd.Add("d")
	require.NoError(t, st.segs.Update("seg", toAdd, set.NewSet(), 10))

	after, err := st.segs.ChangesSince("seg", -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "d"}, after.Added)
	assert.Equal(t, int64(10), after.Till)
}

// TestSegmentChangesContractForControllerPath checks that the segmentChanges controller contract
// maps ChangesSince results to 200 or 404. The controller package imports storage, so this test
// checks the storage results directly.
func TestSegmentChangesContractForControllerPath(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{
		Status:         redisstate.BootValid,
		SegmentCursors: map[string]int64{"seg": 9},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 4, "b": 9},
			Removed:      map[string]int64{"c": 5},
			ChangeNumber: 9,
		}},
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	// Restored segment: since -1 returns the full active set with no tombstones.
	payload, err := st.segs.ChangesSince("seg", -1)
	require.NoError(t, err)
	assert.NotErrorIs(t, err, ErrSegmentNotFound)
	assert.ElementsMatch(t, []string{"a", "b"}, payload.Added)
	assert.Empty(t, payload.Removed)
	assert.Equal(t, int64(-1), payload.Since)
	assert.Equal(t, int64(9), payload.Till)

	// An unknown segment is the case that maps to 404.
	_, err = st.segs.ChangesSince("missing", -1)
	assert.ErrorIs(t, err, ErrSegmentNotFound)

	// After one cloud delta sync, the since -1 payload still holds the full set.
	toAdd := set.NewSet()
	toAdd.Add("d")
	require.NoError(t, st.segs.Update("seg", toAdd, set.NewSet(), 10))
	after, err := st.segs.ChangesSince("seg", -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "d"}, after.Added)
	assert.Empty(t, after.Removed)
	assert.Equal(t, int64(10), after.Till)
}

// TestRedisStateSourceReadsRestoredBolt checks that NewRedisStateSource reads the restored Bolt
// through fresh storages.
func TestRedisStateSourceReadsRestoredBolt(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{
		Status: redisstate.BootValid,
		Flags: []dtos.SplitDTO{
			{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"},
			{Name: "f2", ChangeNumber: 4, Status: "ARCHIVED", TrafficTypeName: "ttt"},
		},
		RuleBasedSegments:      []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}},
		FlagCursor:             4,
		RuleBasedSegmentCursor: 5,
		SegmentCursors:         map[string]int64{"seg": 7},
		Segments: []redisstate.SegmentState{{
			Name:         "seg",
			Active:       map[string]int64{"a": 6, "b": 7},
			Removed:      map[string]int64{"c": 5},
			ChangeNumber: 7,
		}},
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	logger := logging.NewLogger(nil)
	splits := NewProxySplitStorage(st.db, logger, flagsets.NewFlagSetFilter(nil), true)
	rbs := NewProxyRuleBasedSegmentsStorage(st.db, logger, true)
	segs := NewProxySegmentStorage(st.db, logger, true)
	require.Nil(t, ApplyCommittedCursors(state, splits, rbs, segs))

	src := NewRedisStateSource(splits, rbs, segs)

	flags, flagCN, err := src.Flags()
	require.NoError(t, err)
	assert.Equal(t, int64(4), flagCN)
	assert.ElementsMatch(t, []string{"f1", "f2"}, splitNames(flags))

	rbAll, rbCN, err := src.RuleBasedSegments()
	require.NoError(t, err)
	assert.Equal(t, int64(5), rbCN)
	require.Len(t, rbAll, 1)
	assert.Equal(t, "rb1", rbAll[0].Name)

	var segStates []redisstate.SegmentState
	require.NoError(t, src.Segments(func(s redisstate.SegmentState) error {
		segStates = append(segStates, s)
		return nil
	}))
	require.Len(t, segStates, 1)
	assert.Equal(t, "seg", segStates[0].Name)
	assert.Equal(t, int64(7), segStates[0].ChangeNumber)
	assert.Equal(t, map[string]int64{"a": 6, "b": 7}, segStates[0].Active)
	assert.Equal(t, map[string]int64{"c": 5}, segStates[0].Removed)

	assert.Equal(t, 0, st.writes.n)
}

// Restoring through the sink (entities first, then one segment at a time) serves the same data as
// the all-at-once restore, and never writes to redis.
func TestRestoreSinkMatchesBulkRestore(t *testing.T) {
	st := newRestoreStores(t)
	flags := []dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"}}
	rbs := []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}}
	state := redisstate.BootState{
		Status: redisstate.BootValid, FlagCursor: 3, RuleBasedSegmentCursor: 5,
		SegmentCursors: map[string]int64{"seg1": 7, "seg2": 8}, SegmentCount: 2,
	}

	sink := NewRedisRestoreSink(st.splits, st.rbs, st.segs)
	require.Nil(t, sink.Entities(flags, rbs))
	require.Nil(t, sink.Segment(redisstate.SegmentState{
		Name:         "seg1",
		Active:       map[string]int64{"a": 6, "b": 7},
		Removed:      map[string]int64{"c": 5},
		ChangeNumber: 7,
	}))
	require.Nil(t, sink.Segment(redisstate.SegmentState{
		Name:         "seg2",
		Active:       map[string]int64{"x": 8},
		Removed:      map[string]int64{},
		ChangeNumber: 8,
	}))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))
	assert.Equal(t, 0, st.writes.n)

	flagsView, err := st.splits.ChangesSince(-1, nil)
	require.Nil(t, err)
	assert.Equal(t, []string{"f1"}, splitNames(flagsView.Splits))
	rbView, err := st.rbs.ChangesSince(-1)
	require.Nil(t, err)
	require.Len(t, rbView.RuleBasedSegments, 1)
	seg1, err := st.segs.ChangesSince("seg1", 4)
	require.Nil(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, seg1.Added)
	assert.Equal(t, []string{"c"}, seg1.Removed)
	seg2, err := st.segs.ChangesSince("seg2", -1)
	require.Nil(t, err)
	assert.ElementsMatch(t, []string{"x"}, seg2.Added)
	assert.Equal(t, int64(8), seg2.Till)
}

// A Bolt failure while restoring is returned by the sink, not swallowed.
func TestRestoreSinkBoltFailureIsReturned(t *testing.T) {
	logger := logging.NewLogger(nil)
	db := failingBolt{}
	sink := NewRedisRestoreSink(
		NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false),
		NewProxyRuleBasedSegmentsStorage(db, logger, false),
		NewProxySegmentStorage(db, logger, false),
	)
	assert.ErrorIs(t, sink.Entities([]dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE"}}, nil), errDiskFull)
	assert.ErrorIs(t, sink.Segment(redisstate.SegmentState{
		Name:         "seg",
		Active:       map[string]int64{"a": 4},
		ChangeNumber: 4,
	}), errDiskFull)
}

// Needs Redis on localhost:6379 (skips without it). Same shape as the read-only BenchmarkReaderBoot
// in redisstate, but this one reads into a RedisRestoreSink backed by an in-memory Bolt.
// PROXY_BENCH_RETAIN=1 keeps all segments, then restores (the retained path).
// PROXY_BENCH_SEGMENTS and PROXY_BENCH_KEYS set the size (default 5 x 20k).

const benchFingerprint = "restore-bench-fp"

func BenchmarkReaderBootIntoBolt(b *testing.B) {
	segments, keysPerSeg := 5, 20000
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_SEGMENTS")); err == nil && n > 0 {
		segments = n
	}
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_KEYS")); err == nil && n > 0 {
		keysPerSeg = n
	}
	b.Run(fmt.Sprintf("segments=%d_keys=%d", segments, keysPerSeg), func(b *testing.B) {
		benchReaderBootIntoBolt(b, segments, keysPerSeg)
	})
}

func benchReaderBootIntoBolt(b *testing.B, segments, keysPerSeg int) {
	probe := goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DialTimeout: time.Second, MaxRetries: -1})
	pctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	pingErr := probe.Ping(pctx).Err()
	cancel()
	_ = probe.Close()
	if pingErr != nil {
		b.Skipf("redis is not reachable on localhost:6379: %v", pingErr)
	}

	prefix := "t-" + b.Name()
	cfg := &config.RedisConfig{Host: "localhost", Port: 6379, Database: 0, Network: "tcp", Prefix: prefix}
	logger := logging.NewLogger(nil)
	wclient, err := commonsredis.NewRedisClient(cfg, logger)
	require.NoError(b, err)
	resetPrefix := func() {
		raw := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
		defer func() { _ = raw.Close() }()
		ctx := context.Background()
		keys, kerr := raw.Keys(ctx, prefix+".*").Result()
		require.NoError(b, kerr)
		if len(keys) > 0 {
			require.NoError(b, raw.Del(ctx, keys...).Err())
		}
	}
	resetPrefix()
	b.Cleanup(resetPrefix)
	wkeys, err := redisstate.NewKeys(wclient.Prefix(), "bench-key", "")
	require.NoError(b, err)
	w, err := redisstate.NewWriter(wclient, wkeys, benchFingerprint, redisstate.DefaultOptions(), logger)
	require.NoError(b, err)
	require.NoError(b, w.Start(nil))
	b.Cleanup(w.Stop)

	segNames := make([]string, segments)
	matchers := make([]dtos.MatcherDTO, segments)
	for s := range segNames {
		segNames[s] = fmt.Sprintf("seg-%03d", s)
		matchers[s] = dtos.MatcherDTO{UserDefinedSegment: &dtos.UserDefinedSegmentMatcherDataDTO{SegmentName: segNames[s]}}
	}
	flags := make([]dtos.SplitDTO, 0, 20)
	for i := 0; i < cap(flags); i++ {
		flags = append(flags, dtos.SplitDTO{Name: fmt.Sprintf("flag-%03d", i), Status: "ACTIVE", ChangeNumber: int64(i + 1)})
	}
	flags[0].Conditions = []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: matchers}}}
	w.PutFlags(flags, nil, int64(len(flags)))
	for s := 0; s < segments; s++ {
		keys := make([]string, keysPerSeg)
		for k := range keys {
			keys[k] = fmt.Sprintf("s%03d-k%07d", s, k)
		}
		w.PutSegment(segNames[s], keys, nil, 10)
	}
	deadline := time.Now().Add(10 * time.Minute)
	for w.Status().PendingChanges > 0 {
		require.True(b, time.Now().Before(deadline), "writer did not drain its queue")
		time.Sleep(5 * time.Millisecond)
	}

	rc, err := redisstate.NewReadClient(cfg)
	require.NoError(b, err)
	b.Cleanup(func() { _ = rc.Close() })
	rkeys, err := redisstate.NewKeys(rc.Prefix(), "bench-key", "")
	require.NoError(b, err)
	reader, err := redisstate.NewReader(rc, rkeys, benchFingerprint)
	require.NoError(b, err)

	readOnce := func() {
		db, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
		require.NoError(b, err)
		defer func() { _ = db.Discard() }()
		sink := NewRedisRestoreSink(
			NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false),
			NewProxyRuleBasedSegmentsStorage(db, logger, false),
			NewProxySegmentStorage(db, logger, false),
		)
		var state redisstate.BootState
		if os.Getenv("PROXY_BENCH_RETAIN") == "1" {
			// Retain path: keep every segment payload, then restore from the retained state.
			state, err = reader.Read(context.Background(), func(redisstate.SegmentState) error { return nil })
			require.NoError(b, err)
			require.NoError(b, sink.Entities(state.Flags, state.RuleBasedSegments))
			for _, st := range state.Segments {
				require.NoError(b, sink.Segment(st))
			}
			state.SegmentCount = len(state.Segments)
		} else {
			state, err = reader.ReadInto(context.Background(), sink)
		}
		require.NoError(b, err)
		require.Equal(b, redisstate.BootValid, state.Status, state.Reason)
		require.Equal(b, segments, state.SegmentCount)
	}
	readOnce()

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	peak := base.HeapAlloc
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak {
					peak = ms.HeapAlloc
				}
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		readOnce()
	}
	b.StopTimer()
	close(stop)
	<-done

	var delta float64
	if peak > base.HeapAlloc {
		delta = float64(peak - base.HeapAlloc)
	}
	b.ReportMetric(delta, "peak-heap-B")
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) == nil {
		rss := uint64(ru.Maxrss)
		if runtime.GOOS != "darwin" {
			rss *= 1024
		}
		b.ReportMetric(float64(rss), "peak-rss-B")
	}
}

// Two entries with the same name must fail before anything is written: the batched update is only equivalent to
// per change number updates when each name appears once.
func TestRestoreSinkRejectsDuplicateFlagNames(t *testing.T) {
	st := newRestoreStores(t)
	sink := NewRedisRestoreSink(st.splits, st.rbs, st.segs)
	flags := []dtos.SplitDTO{
		{Name: "dup", ChangeNumber: 1, Status: "ACTIVE", TrafficTypeName: "ttt"},
		{Name: "dup", ChangeNumber: 2, Status: "ARCHIVED", TrafficTypeName: "ttt"},
	}
	err := sink.Entities(flags, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDuplicateEntityName)
	assert.Nil(t, st.splits.FetchMany([]string{"dup"})["dup"])
}

func TestRestoreSinkRejectsDuplicateRuleBasedSegmentNames(t *testing.T) {
	st := newRestoreStores(t)
	sink := NewRedisRestoreSink(st.splits, st.rbs, st.segs)
	rbs := []dtos.RuleBasedSegmentDTO{
		{Name: "dup", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "dup", ChangeNumber: 2, Status: "ARCHIVED"},
	}
	err := sink.Entities(nil, rbs)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDuplicateEntityName)
	assert.Nil(t, st.rbs.FetchMany([]string{"dup"})["dup"])
}

func TestRestoreSinkRejectsKeyBothActiveAndRemoved(t *testing.T) {
	st := newRestoreStores(t)
	sink := NewRedisRestoreSink(st.splits, st.rbs, st.segs)
	err := sink.Segment(redisstate.SegmentState{
		Name:         "s",
		Active:       map[string]int64{"k": 1},
		Removed:      map[string]int64{"k": 2},
		ChangeNumber: 2,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSegmentKeyConflict)
	cn, cnErr := st.segs.ChangeNumber("s")
	require.NoError(t, cnErr)
	assert.Equal(t, int64(-1), cn, "no segment item must be stored")
}

func TestRestoreSinkRejectsDuplicateSegmentNames(t *testing.T) {
	st := newRestoreStores(t)
	sink := NewRedisRestoreSink(st.splits, st.rbs, st.segs)
	require.NoError(t, sink.Segment(redisstate.SegmentState{
		Name:         "dup",
		Active:       map[string]int64{"a": 1},
		ChangeNumber: 1,
	}))
	err := sink.Segment(redisstate.SegmentState{Name: "dup", Active: map[string]int64{"b": 2}, ChangeNumber: 2})
	require.Error(t, err)
	assert.ErrorIs(t, err, errDuplicateEntityName)
	cn, cnErr := st.segs.ChangeNumber("dup")
	require.NoError(t, cnErr)
	assert.Equal(t, int64(1), cn, "the first segment must stay untouched by the rejected repeat")
}

func TestRestoreFromRedisStateIgnoresNonValidBoot(t *testing.T) {
	st := newRestoreStores(t)
	state := redisstate.BootState{
		Status: redisstate.BootCorrupt,
		Flags:  []dtos.SplitDTO{{Name: "f", ChangeNumber: 1, Status: "ACTIVE", TrafficTypeName: "ttt"}},
	}
	require.NoError(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	assert.Nil(t, st.splits.FetchMany([]string{"f"})["f"])
}

// notSegmentCollection is a SegmentChangesCollection that is not Bolt-backed, so restore cannot
// write to it.
type notSegmentCollection struct {
	persistent.SegmentChangesCollection
}

func TestRestoreSegmentNonBoltCollectionIsDetectable(t *testing.T) {
	s := &ProxySegmentStorageImpl{db: notSegmentCollection{}}
	err := restoreSegment(s, redisstate.SegmentState{Name: "seg", Active: map[string]int64{"a": 1}, ChangeNumber: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSegmentCollectionNotRestorable)
}

func TestApplyCommittedCursorsNegativeFlagCursorIsSkipped(t *testing.T) {
	st := newRestoreStores(t)

	state := redisstate.BootState{
		Status:                 redisstate.BootValid,
		Flags:                  []dtos.SplitDTO{{Name: "f1", ChangeNumber: 9, Status: "ACTIVE", TrafficTypeName: "ttt"}},
		FlagCursor:             -1,
		RuleBasedSegmentCursor: -1,
	}
	require.Nil(t, restoreFromRedisState(state, st.splits, st.rbs, st.segs))
	require.Nil(t, ApplyCommittedCursors(state, st.splits, st.rbs, st.segs))

	flagCN, err := st.splits.ChangeNumber()
	require.Nil(t, err)
	assert.Equal(t, int64(9), flagCN, "a negative flag cursor must not overwrite the restored change number")
}
