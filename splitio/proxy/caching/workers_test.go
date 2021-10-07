package caching

import (
	"testing"

	storageMocks "github.com/splitio/go-split-commons/v4/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"

	cacheMocks "github.com/splitio/gincache/mocks"
)

func TestCacheAwareSplitSync(t *testing.T) {
	var cn int64 = -1

	splitSyncMock := &splitUpdaterMock{
		SynchronizeSplitsCall: func(*int64, bool) ([]string, error) { return nil, nil },
		LocalKillCall:         func(string, string, int64) {},
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

	css.SynchronizeSplits(nil, false)

	splitSyncMock.SynchronizeSplitsCall = func(*int64, bool) ([]string, error) {
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

	css.SynchronizeSplits(nil, false)
	if calls != 1 {
		t.Error("should have flushed splits once")
	}

	css.LocalKill("someSplit", "off", 123)
	if calls != 2 {
		t.Error("should have flushed again after a local kill")
	}

	// Test that going from cn > -1 to cn == -1 purges
	cn = 123
	splitSyncMock.SynchronizeSplitsCall = func(*int64, bool) ([]string, error) {
		cn = -1
		return nil, nil
	}
	css.SynchronizeSplits(nil, false)
	if calls != 3 {
		t.Error("should have flushed splits once", calls)
	}
}

func TestCacheAwareSegmentSync(t *testing.T) {
	cns := map[string]int64{"segment1": 0}

	segmentSyncMock := &segmentUpdaterMock{
		SynchronizeSegmentCall:  func(string, *int64, bool) error { return nil },
		SynchronizeSegmentsCall: func(bool) error { return nil },
	}
	cacheFlusherMock := &cacheMocks.CacheFlusherMock{
		EvictBySurrogateCall: func(string) { t.Error("nothing should be evicted") },
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

	css.SynchronizeSegment("segment1", nil, false)

	segmentSyncMock.SynchronizeSegmentCall = func(name string, c *int64, q bool) error {
		cns[name]++
		return nil
	}

	calls := 0
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment1") {
			t.Error("wrong surrogate")
		}
		calls++
	}

	// SynchronizeSegment

	css.SynchronizeSegment("segment1", nil, false)
	if calls != 1 {
		t.Error("should have flushed splits once")
	}

	// Test that going from cn > -1 to cn == -1 purges
	cns["segment1"] = 123
	segmentSyncMock.SynchronizeSegmentCall = func(name string, s *int64, q bool) error {
		cns[name] = -1
		return nil
	}
	css.SynchronizeSegment("segment1", nil, false)
	if calls != 2 {
		t.Error("should have flushed splits once", calls)
	}

	// SynchronizeSegments

	// Case 1: updated CN
	cns["segment2"] = 0
	segmentSyncMock.SynchronizeSegmentsCall = func(bool) error {
		cns["segment2"]++ //increment one, should cause eviction of this segment
		return nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment2") {
			t.Error("wrong surrogate")
		}
		calls++
	}

	css.SynchronizeSegments(false)
	if calls != 3 {
		t.Error("should have flushed segments twice")
	}

	// Case 2: added segment
	segmentSyncMock.SynchronizeSegmentsCall = func(bool) error {
		cns["segment3"]++ //increment one, should cause eviction of this segment
		return nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment3") {
			t.Error("wrong surrogate")
		}
		calls++
	}

	css.SynchronizeSegments(false)
	if calls != 4 {
		t.Error("should have flushed segments twice")
	}

	// Case 3: deleted segment
	segmentSyncMock.SynchronizeSegmentsCall = func(bool) error {
		delete(cns, "segment3")
		return nil
	}

	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment3") {
			t.Error("wrong surrogate", key)
		}
		calls++
	}

	css.SynchronizeSegments(false)
	if calls != 5 {
		t.Error("should have flushed segments twice")
	}

	// all keys deleted & segment till is now -1
	cacheFlusherMock.EvictBySurrogateCall = func(key string) {
		if key != MakeSurrogateForSegmentChanges("segment2") {
			t.Error("wrong surrogate", key)
		}
		calls++
	}
	cns["segment2"] = 123
	segmentSyncMock.SynchronizeSegmentsCall = func(bool) error {
		cns["segment2"] = -1
		return nil
	}
	css.SynchronizeSegments(false)
	if calls != 6 {
		t.Error("should have flushed segments twice")
	}
}

type splitUpdaterMock struct {
	SynchronizeSplitsCall func(till *int64, requestNoCache bool) ([]string, error)
	LocalKillCall         func(splitName string, defaultTreatment string, changeNumber int64)
}

func (s *splitUpdaterMock) SynchronizeSplits(till *int64, requestNoCache bool) ([]string, error) {
	return s.SynchronizeSplitsCall(till, requestNoCache)
}

func (s *splitUpdaterMock) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	s.LocalKillCall(splitName, defaultTreatment, changeNumber)
}

type segmentUpdaterMock struct {
	SynchronizeSegmentCall  func(name string, till *int64, requestNoCache bool) error
	SynchronizeSegmentsCall func(requestNoCache bool) error
	SegmentNamesCall        func() []interface{}
	IsSegmentCachedCall     func(segmentName string) bool
}

func (s *segmentUpdaterMock) SynchronizeSegment(name string, till *int64, requestNoCache bool) error {
	return s.SynchronizeSegmentCall(name, till, requestNoCache)
}

func (s *segmentUpdaterMock) SynchronizeSegments(requestNoCache bool) error {
	return s.SynchronizeSegmentsCall(requestNoCache)
}

func (s *segmentUpdaterMock) SegmentNames() []interface{} {
	return s.SegmentNamesCall()
}

func (s *segmentUpdaterMock) IsSegmentCached(segmentName string) bool {
	return s.IsSegmentCachedCall(segmentName)
}
