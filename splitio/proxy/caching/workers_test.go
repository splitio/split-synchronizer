package caching

import (
	"testing"

	"github.com/splitio/go-split-commons/v5/dtos"
	storageMocks "github.com/splitio/go-split-commons/v5/storage/mocks"
	"github.com/splitio/go-split-commons/v5/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v5/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/datastructures/set"

	cacheMocks "github.com/splitio/gincache/mocks"
)

func TestCacheAwareSplitSync(t *testing.T) {
	var cn int64 = -1

	splitSyncMock := &splitUpdaterMock{
		SynchronizeFeatureFlagsCall: func(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) { return nil, nil },
		SynchronizeSplitsCall:       func(*int64) (*split.UpdateResult, error) { return nil, nil },
		LocalKillCall:               func(string, string, int64) {},
	}
	cacheFlusherMock := &cacheMocks.CacheFlusherMock{
		EvictBySurrogateCall: func(string) { t.Error("nothing should be evicted") },
	}

	css := CacheAwareSplitSynchronizer{
		splitStorage: &storageMocks.MockSplitStorage{
			ChangeNumberCall: func() (int64, error) { return cn, nil },
		},
		wrapped:      splitSyncMock,
		cacheFlusher: cacheFlusherMock,
	}

	css.SynchronizeSplits(nil)

	splitSyncMock.SynchronizeSplitsCall = func(*int64) (*split.UpdateResult, error) {
		cn++
		return nil, nil
	}

	calls := 0
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != SplitSurrogate {
			t.Error("wrong surrogate")
		}
		calls++
	}

	css.SynchronizeSplits(nil)
	if calls != 1 {
		t.Error("should have flushed splits once")
	}

	css.LocalKill("someSplit", "off", 123)
	if calls != 2 {
		t.Error("should have flushed again after a local kill")
	}

	// Test that going from cn > -1 to cn == -1 purges
	cn = 123
	splitSyncMock.SynchronizeSplitsCall = func(*int64) (*split.UpdateResult, error) {
		cn = -1
		return nil, nil
	}
	css.SynchronizeSplits(nil)
	if calls != 3 {
		t.Error("should have flushed splits once", calls)
	}
}

func TestCacheAwareSplitSyncFF(t *testing.T) {
	var cn int64 = -1

	splitSyncMock := &splitUpdaterMock{
		SynchronizeFeatureFlagsCall: func(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) { return nil, nil },
		SynchronizeSplitsCall:       func(*int64) (*split.UpdateResult, error) { return nil, nil },
		LocalKillCall:               func(string, string, int64) {},
	}
	cacheFlusherMock := &cacheMocks.CacheFlusherMock{
		EvictBySurrogateCall: func(string) { t.Error("nothing should be evicted") },
	}

	css := CacheAwareSplitSynchronizer{
		splitStorage: &storageMocks.MockSplitStorage{
			ChangeNumberCall: func() (int64, error) { return cn, nil },
		},
		wrapped:      splitSyncMock,
		cacheFlusher: cacheFlusherMock,
	}

	css.SynchronizeFeatureFlags(nil)

	splitSyncMock.SynchronizeFeatureFlagsCall = func(*dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
		cn++
		return nil, nil
	}

	calls := 0
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != SplitSurrogate {
			t.Error("wrong surrogate")
		}
		calls++
	}

	css.SynchronizeFeatureFlags(nil)
	if calls != 1 {
		t.Error("should have flushed splits once")
	}

	css.LocalKill("someSplit", "off", 123)
	if calls != 2 {
		t.Error("should have flushed again after a local kill")
	}

	// Test that going from cn > -1 to cn == -1 purges
	cn = 123
	splitSyncMock.SynchronizeFeatureFlagsCall = func(*dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
		cn = -1
		return nil, nil
	}
	css.SynchronizeFeatureFlags(nil)
	if calls != 3 {
		t.Error("should have flushed splits once", calls)
	}
}

func TestCacheAwareSegmentSync(t *testing.T) {
	cns := map[string]int64{"segment1": 0}

	segmentSyncMock := &segmentUpdaterMock{
		SynchronizeSegmentCall:  func(string, *int64) (*segment.UpdateResult, error) { return &segment.UpdateResult{}, nil },
		SynchronizeSegmentsCall: func() (map[string]segment.UpdateResult, error) { return nil, nil },
	}
	cacheFlusherMock := &cacheMocks.CacheFlusherMock{
		EvictBySurrogateCall: func(string) { t.Error("nothing should be evicted") },
		EvictCall:            func(string) { t.Errorf("nothing should be evicted") },
	}

	css := CacheAwareSegmentSynchronizer{
		splitStorage: &storageMocks.MockSplitStorage{
			SegmentNamesCall: func() *set.ThreadUnsafeSet {
				s := set.NewSet()
				for k := range cns {
					s.Add(k)
				}
				return s
			},
		},
		segmentStorage: &storageMocks.MockSegmentStorage{
			ChangeNumberCall: func(s string) (int64, error) {
				cn, _ := cns[s]
				return cn, nil
			},
		},
		wrapped:      segmentSyncMock,
		cacheFlusher: cacheFlusherMock,
	}

	css.SynchronizeSegment("segment1", nil)

	segmentSyncMock.SynchronizeSegmentCall = func(name string, c *int64) (*segment.UpdateResult, error) {
		return &segment.UpdateResult{UpdatedKeys: []string{"k1"}, NewChangeNumber: 2}, nil
	}

	evictBySurrogateCalls := 0
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment1") {
			t.Error("wrong surrogate")
		}
		evictBySurrogateCalls++
	}
	cacheFlusherMock.EvictCall = func(key string) {
		if key != "/api/mySegments/k1" && key != "gzip::/api/mySegments/k1" {
			t.Error("incorrect mysegments entry purged: ", key)
		}
	}

	// SynchronizeSegment

	css.SynchronizeSegment("segment1", nil)
	if evictBySurrogateCalls != 1 {
		t.Error("should have flushed splits once. Got", evictBySurrogateCalls)
	}

	// Test that going from cn > -1 to cn == -1 purges
	cns["segment1"] = 123
	segmentSyncMock.SynchronizeSegmentCall = func(name string, s *int64) (*segment.UpdateResult, error) {
		return &segment.UpdateResult{UpdatedKeys: []string{"k1"}, NewChangeNumber: -1}, nil
	}
	css.SynchronizeSegment("segment1", nil)
	if evictBySurrogateCalls != 2 {
		t.Error("should have flushed splits once", evictBySurrogateCalls)
	}

	// SynchronizeSegments

	// Case 1: updated CN
	cns["segment2"] = 0
	segmentSyncMock.SynchronizeSegmentsCall = func() (map[string]segment.UpdateResult, error) {
		return map[string]segment.UpdateResult{"segment2": {UpdatedKeys: []string{"k1"}, NewChangeNumber: 1}}, nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment2") {
			t.Error("wrong surrogate")
		}
		evictBySurrogateCalls++
	}

	css.SynchronizeSegments()
	if evictBySurrogateCalls != 3 {
		t.Error("should have flushed segments twice")
	}

	// Case 2: added segment
	cns["segment3"] = 2
	segmentSyncMock.SynchronizeSegmentsCall = func() (map[string]segment.UpdateResult, error) {
		return map[string]segment.UpdateResult{"segment3": {UpdatedKeys: []string{"k1"}, NewChangeNumber: 3}}, nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment3") {
			t.Error("wrong surrogate")
		}
		evictBySurrogateCalls++
	}

	css.SynchronizeSegments()
	if evictBySurrogateCalls != 4 {
		t.Error("should have flushed segments twice")
	}

	// Case 3: deleted segment
	segmentSyncMock.SynchronizeSegmentsCall = func() (map[string]segment.UpdateResult, error) {
		return map[string]segment.UpdateResult{"segment3": {UpdatedKeys: []string{"k1"}, NewChangeNumber: -1}}, nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment3") {
			t.Error("wrong surrogate", key)
		}
		evictBySurrogateCalls++
	}

	css.SynchronizeSegments()
	if evictBySurrogateCalls != 5 {
		t.Error("should have flushed segments 5 times: ", evictBySurrogateCalls)
	}

	// all keys deleted & segment till is now -1
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment2") {
			t.Error("wrong surrogate", key)
		}
		evictBySurrogateCalls++
	}
	cns["segment2"] = 123
	segmentSyncMock.SynchronizeSegmentsCall = func() (map[string]segment.UpdateResult, error) {
		return map[string]segment.UpdateResult{"segment2": {UpdatedKeys: []string{"k1"}, NewChangeNumber: -1}}, nil
	}
	css.SynchronizeSegments()
	if evictBySurrogateCalls != 6 {
		t.Error("should have flushed segments twice")
	}
}

type splitUpdaterMock struct {
	SynchronizeFeatureFlagsCall func(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error)
	SynchronizeSplitsCall       func(till *int64) (*split.UpdateResult, error)
	LocalKillCall               func(splitName string, defaultTreatment string, changeNumber int64)
}

func (s *splitUpdaterMock) SynchronizeSplits(till *int64) (*split.UpdateResult, error) {
	return s.SynchronizeSplitsCall(till)
}

func (s *splitUpdaterMock) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	s.LocalKillCall(splitName, defaultTreatment, changeNumber)
}

func (s *splitUpdaterMock) SynchronizeFeatureFlags(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
	return s.SynchronizeFeatureFlagsCall(ffChange)
}

type segmentUpdaterMock struct {
	SynchronizeSegmentCall  func(name string, till *int64) (*segment.UpdateResult, error)
	SynchronizeSegmentsCall func() (map[string]segment.UpdateResult, error)
	SegmentNamesCall        func() []interface{}
	IsSegmentCachedCall     func(segmentName string) bool
}

func (s *segmentUpdaterMock) SynchronizeSegment(name string, till *int64) (*segment.UpdateResult, error) {
	return s.SynchronizeSegmentCall(name, till)
}

func (s *segmentUpdaterMock) SynchronizeSegments() (map[string]segment.UpdateResult, error) {
	return s.SynchronizeSegmentsCall()
}

func (s *segmentUpdaterMock) SegmentNames() []interface{} {
	return s.SegmentNamesCall()
}

func (s *segmentUpdaterMock) IsSegmentCached(segmentName string) bool {
	return s.IsSegmentCachedCall(segmentName)
}
