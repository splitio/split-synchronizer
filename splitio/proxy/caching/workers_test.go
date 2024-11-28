package caching

import (
	"testing"

	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-split-commons/v6/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v6/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCacheAwareSplitSyncNoChanges(t *testing.T) {
	var splitSyncMock mocks.SplitUpdaterMock
	splitSyncMock.On("SynchronizeSplits", (*int64)(nil)).Return((*split.UpdateResult)(nil), error(nil))
	var cacheFlusherMock mocks.CacheFlusherMock
	var storageMock mocks.SplitStorageMock
	storageMock.On("ChangeNumber").Return(int64(-1), error(nil))

	css := CacheAwareSplitSynchronizer{
		splitStorage: &storageMock,
		wrapped:      &splitSyncMock,
		cacheFlusher: &cacheFlusherMock,
	}

	res, err := css.SynchronizeSplits(nil)
	assert.Nil(t, err)
	assert.Nil(t, res)

	splitSyncMock.AssertExpectations(t)
	cacheFlusherMock.AssertExpectations(t)
	storageMock.AssertExpectations(t)
}

func TestCacheAwareSplitSyncChanges(t *testing.T) {
	var splitSyncMock mocks.SplitUpdaterMock
	splitSyncMock.On("SynchronizeSplits", (*int64)(nil)).Return((*split.UpdateResult)(nil), error(nil)).Times(2)

	var cacheFlusherMock mocks.CacheFlusherMock
	cacheFlusherMock.On("EvictBySurrogate", SplitSurrogate).Times(3)

	var storageMock mocks.SplitStorageMock
	storageMock.On("ChangeNumber").Return(int64(-1), error(nil)).Once()
	storageMock.On("ChangeNumber").Return(int64(1), error(nil)).Once()

	css := CacheAwareSplitSynchronizer{
		splitStorage: &storageMock,
		wrapped:      &splitSyncMock,
		cacheFlusher: &cacheFlusherMock,
	}

	res, err := css.SynchronizeSplits(nil)
	assert.Nil(t, err)
	assert.Nil(t, res)

	splitSyncMock.On("LocalKill", "someSplit", "off", int64(123)).Return(nil).Once()
	css.LocalKill("someSplit", "off", 123)

	// Test that going from cn > -1 to cn == -1 purges (can happen if the environment if wiped of splits)
	storageMock.On("ChangeNumber").Return(int64(123), error(nil)).Once()
	storageMock.On("ChangeNumber").Return(int64(-1), error(nil)).Once()
	res, err = css.SynchronizeSplits(nil)
	assert.Nil(t, err)
	assert.Nil(t, res)

	splitSyncMock.AssertExpectations(t)
	cacheFlusherMock.AssertExpectations(t)
	storageMock.AssertExpectations(t)
}

func TestCacheAwareSplitSyncChangesNewMethod(t *testing.T) {

	// This test is used to test the new method. Eventually commons should be cleaned in order to have a single method for split-synchronization.
	// when that happens, either this or the previous test shold be removed
	var splitSyncMock mocks.SplitUpdaterMock
	splitSyncMock.On("SynchronizeFeatureFlags", (*dtos.SplitChangeUpdate)(nil)).Return((*split.UpdateResult)(nil), error(nil)).Times(2)

	var cacheFlusherMock mocks.CacheFlusherMock
	cacheFlusherMock.On("EvictBySurrogate", SplitSurrogate).Times(2)

	var storageMock mocks.SplitStorageMock
	storageMock.On("ChangeNumber").Return(int64(-1), error(nil)).Once()
	storageMock.On("ChangeNumber").Return(int64(1), error(nil)).Once()

	css := CacheAwareSplitSynchronizer{
		splitStorage: &storageMock,
		wrapped:      &splitSyncMock,
		cacheFlusher: &cacheFlusherMock,
	}

	res, err := css.SynchronizeFeatureFlags(nil)
	assert.Nil(t, err)
	assert.Nil(t, res)

	// Test that going from cn > -1 to cn == -1 purges (can happen if the environment if wiped of splits)
	storageMock.On("ChangeNumber").Return(int64(123), error(nil)).Once()
	storageMock.On("ChangeNumber").Return(int64(-1), error(nil)).Once()
	res, err = css.SynchronizeFeatureFlags(nil)
	assert.Nil(t, err)
	assert.Nil(t, res)

	splitSyncMock.AssertExpectations(t)
	cacheFlusherMock.AssertExpectations(t)
	storageMock.AssertExpectations(t)
}

func TestCacheAwareSegmentSyncNoChanges(t *testing.T) {
	var segmentUpdater mocks.SegmentUpdaterMock
	segmentUpdater.On("SynchronizeSegment", "segment1", (*int64)(nil)).Return(&segment.UpdateResult{}, nil).Once()

	var splitStorage mocks.SplitStorageMock
	var cacheFlusher mocks.CacheFlusherMock
	var segmentStorage mocks.SegmentStorageMock
	segmentStorage.On("ChangeNumber", "segment1").Return(int64(0), nil).Once()

	css := CacheAwareSegmentSynchronizer{
		splitStorage:   &splitStorage,
		segmentStorage: &segmentStorage,
		wrapped:        &segmentUpdater,
		cacheFlusher:   &cacheFlusher,
	}

	res, err := css.SynchronizeSegment("segment1", nil)
	assert.Nil(t, err)
	assert.Equal(t, &segment.UpdateResult{}, res)

	segmentUpdater.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
	splitStorage.AssertExpectations(t)
	cacheFlusher.AssertExpectations(t)
}

func TestCacheAwareSegmentSyncSingle(t *testing.T) {
	var segmentUpdater mocks.SegmentUpdaterMock
	segmentUpdater.On("SynchronizeSegment", "segment1", (*int64)(nil)).Return(&segment.UpdateResult{
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: 2,
	}, nil).Once()

	var splitStorage mocks.SplitStorageMock

	var cacheFlusher mocks.CacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MakeSurrogateForSegmentChanges("segment1")).Times(2)
	cacheFlusher.On("Evict", "/api/mySegments/k1").Times(2)
	cacheFlusher.On("Evict", "gzip::/api/mySegments/k1").Times(2)
	cacheFlusher.On("EvictBySurrogate", MembershipsSurrogate).Times(2)

	var segmentStorage mocks.SegmentStorageMock
	segmentStorage.On("ChangeNumber", "segment1").Return(int64(0), nil).Once()

	css := CacheAwareSegmentSynchronizer{
		splitStorage:   &splitStorage,
		segmentStorage: &segmentStorage,
		wrapped:        &segmentUpdater,
		cacheFlusher:   &cacheFlusher,
	}

	res, err := css.SynchronizeSegment("segment1", nil)
	assert.Nil(t, err)
	assert.Equal(t, &segment.UpdateResult{UpdatedKeys: []string{"k1"}, NewChangeNumber: 2}, res)

	//	// Test that going from cn > -1 to cn == -1 purges
	segmentStorage.On("ChangeNumber", "segment1").Return(int64(123), nil).Once()
	segmentUpdater.On("SynchronizeSegment", "segment1", (*int64)(nil)).Return(&segment.UpdateResult{
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: -1,
	}, nil).Once()
	res, err = css.SynchronizeSegment("segment1", nil)
	assert.Nil(t, err)
	assert.Equal(t, &segment.UpdateResult{UpdatedKeys: []string{"k1"}, NewChangeNumber: -1}, res)

	segmentUpdater.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
	splitStorage.AssertExpectations(t)
	cacheFlusher.AssertExpectations(t)
}

func TestCacheAwareSegmentSyncAllSegments(t *testing.T) {
	var segmentUpdater mocks.SegmentUpdaterMock
	segmentUpdater.On("SynchronizeSegments").Return(map[string]segment.UpdateResult{"segment2": {
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: 1,
	}}, nil).Once()

	var splitStorage mocks.SplitStorageMock
	splitStorage.On("SegmentNames").Return(set.NewSet("segment2")).Once()

	var cacheFlusher mocks.CacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MakeSurrogateForSegmentChanges("segment2")).Times(1)
	cacheFlusher.On("Evict", "/api/mySegments/k1").Times(3)
	cacheFlusher.On("Evict", "gzip::/api/mySegments/k1").Times(3)
	cacheFlusher.On("EvictBySurrogate", MembershipsSurrogate).Times(3)

	var segmentStorage mocks.SegmentStorageMock
	segmentStorage.On("ChangeNumber", "segment2").Return(int64(0), nil).Once()

	css := CacheAwareSegmentSynchronizer{
		splitStorage:   &splitStorage,
		segmentStorage: &segmentStorage,
		wrapped:        &segmentUpdater,
		cacheFlusher:   &cacheFlusher,
	}

	// Case 1: updated CN
	res, err := css.SynchronizeSegments()
	assert.Nil(t, err)
	assert.Equal(t, map[string]segment.UpdateResult{"segment2": {UpdatedKeys: []string{"k1"}, NewChangeNumber: 1}}, res)

	// Case 2: added segment
	segmentStorage.On("ChangeNumber", "segment3").Return(int64(2), nil).Times(2) // for next test as well
	segmentUpdater.On("SynchronizeSegments").Return(map[string]segment.UpdateResult{"segment3": {
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: 3,
	}}, nil).Once()
	cacheFlusher.On("EvictBySurrogate", MakeSurrogateForSegmentChanges("segment3")).Times(2) // for next test as well
	splitStorage.On("SegmentNames").Return(set.NewSet("segment3")).Times(2)                  // for next test as well

	res, err = css.SynchronizeSegments()
	assert.Nil(t, err)
	assert.Equal(t, map[string]segment.UpdateResult{"segment3": {UpdatedKeys: []string{"k1"}, NewChangeNumber: 3}}, res)

	//	// Case 3: deleted segment
	segmentUpdater.On("SynchronizeSegments").Return(map[string]segment.UpdateResult{"segment3": {
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: -1,
	}}, nil).Once()

	res, err = css.SynchronizeSegments()
	assert.Nil(t, err)
	assert.Equal(t, map[string]segment.UpdateResult{"segment3": {UpdatedKeys: []string{"k1"}, NewChangeNumber: -1}}, res)

	segmentUpdater.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
	splitStorage.AssertExpectations(t)
	cacheFlusher.AssertExpectations(t)
}

// CacheAwareLargeSegmentSynchronizer
func TestSynchronizeLargeSegment(t *testing.T) {
	lsName := "largeSegment1"

	var splitStorage mocks.SplitStorageMock
	var cacheFlusher mocks.CacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MembershipsSurrogate).Once()

	var largeSegmentStorage mocks.LargeSegmentStorageMock
	largeSegmentStorage.On("ChangeNumber", lsName).Return(int64(-1)).Once()

	var lsUpdater mocks.LargeSegmentUpdaterMock
	cnToReturn := int64(100)
	lsUpdater.On("SynchronizeLargeSegment", lsName, (*int64)(nil)).Return(&cnToReturn, nil).Once()

	clsSync := CacheAwareLargeSegmentSynchronizer{
		wrapped:             &lsUpdater,
		cacheFlusher:        &cacheFlusher,
		largeSegmentStorage: &largeSegmentStorage,
		splitStorage:        &splitStorage,
	}

	cn, err := clsSync.SynchronizeLargeSegment(lsName, nil)
	if err != nil {
		t.Error("Error should be nil. Actual: ", err)
	}

	if *cn != 100 {
		t.Error("ChangeNumber should be 100. Actual: ", *cn)
	}

	cacheFlusher.AssertExpectations(t)
	largeSegmentStorage.AssertExpectations(t)
	lsUpdater.AssertExpectations(t)
}

func TestSynchronizeLargeSegmentHighestPrevious(t *testing.T) {
	lsName := "largeSegment1"

	var splitStorage mocks.SplitStorageMock
	var cacheFlusher mocks.CacheFlusherMock

	var largeSegmentStorage mocks.LargeSegmentStorageMock
	largeSegmentStorage.On("ChangeNumber", lsName).Return(int64(200)).Once()

	var lsUpdater mocks.LargeSegmentUpdaterMock
	cnToReturn := int64(100)
	lsUpdater.On("SynchronizeLargeSegment", lsName, (*int64)(nil)).Return(&cnToReturn, nil).Once()

	clsSync := CacheAwareLargeSegmentSynchronizer{
		wrapped:             &lsUpdater,
		cacheFlusher:        &cacheFlusher,
		largeSegmentStorage: &largeSegmentStorage,
		splitStorage:        &splitStorage,
	}

	cn, err := clsSync.SynchronizeLargeSegment(lsName, nil)
	if err != nil {
		t.Error("Error should be nil. Actual: ", err)
	}

	if *cn != 100 {
		t.Error("ChangeNumber should be 100. Actual: ", *cn)
	}

	splitStorage.AssertExpectations(t)
	cacheFlusher.AssertExpectations(t)
	largeSegmentStorage.AssertExpectations(t)
	lsUpdater.AssertExpectations(t)
}

func TestSynchronizeLargeSegments(t *testing.T) {
	var splitStorage mocks.SplitStorageMock
	splitStorage.On("LargeSegmentNames").Return(set.NewSet("ls1", "ls2"))

	var cacheFlusher mocks.CacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MembershipsSurrogate).Times(2)

	var cn1 int64 = 100
	var cn2 int64 = 200
	var largeSegmentStorage mocks.LargeSegmentStorageMock
	largeSegmentStorage.On("ChangeNumber", "ls1").Return(cn1 - 50).Once()
	largeSegmentStorage.On("ChangeNumber", "ls2").Return(cn2 - 50).Once()

	var lsUpdater mocks.LargeSegmentUpdaterMock
	result := map[string]*int64{
		"ls1": &cn1,
		"ls2": &cn2,
	}
	lsUpdater.On("SynchronizeLargeSegments").Return(result, nil).Once()

	clsSync := CacheAwareLargeSegmentSynchronizer{
		wrapped:             &lsUpdater,
		cacheFlusher:        &cacheFlusher,
		largeSegmentStorage: &largeSegmentStorage,
		splitStorage:        &splitStorage,
	}

	cn, err := clsSync.SynchronizeLargeSegments()
	if err != nil {
		t.Error("Error should be nil. Actual: ", err)
	}

	if *cn["ls1"] != cn1 {
		t.Error("ChangeNumber should be 100. Actual: ", *cn["ls1"])
	}

	if *cn["ls2"] != cn2 {
		t.Error("ChangeNumber should be 200. Actual: ", *cn["ls2"])
	}

	splitStorage.AssertExpectations(t)
	cacheFlusher.AssertExpectations(t)
	largeSegmentStorage.AssertExpectations(t)
	lsUpdater.AssertExpectations(t)
}
