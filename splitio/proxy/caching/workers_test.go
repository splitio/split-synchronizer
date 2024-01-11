package caching

import (
	"testing"

	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/storage"
	"github.com/splitio/go-split-commons/v5/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v5/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCacheAwareSplitSyncNoChanges(t *testing.T) {
	var splitSyncMock splitUpdaterMock
	splitSyncMock.On("SynchronizeSplits", (*int64)(nil)).Return((*split.UpdateResult)(nil), error(nil))
	var cacheFlusherMock cacheFlusherMock
	var storageMock splitStorageMock
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
	var splitSyncMock splitUpdaterMock
	splitSyncMock.On("SynchronizeSplits", (*int64)(nil)).Return((*split.UpdateResult)(nil), error(nil)).Times(2)

	var cacheFlusherMock cacheFlusherMock
	cacheFlusherMock.On("EvictBySurrogate", SplitSurrogate).Times(3)

	var storageMock splitStorageMock
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
	var splitSyncMock splitUpdaterMock
	splitSyncMock.On("SynchronizeFeatureFlags", (*dtos.SplitChangeUpdate)(nil)).Return((*split.UpdateResult)(nil), error(nil)).Times(2)

	var cacheFlusherMock cacheFlusherMock
	cacheFlusherMock.On("EvictBySurrogate", SplitSurrogate).Times(2)

	var storageMock splitStorageMock
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
	var segmentUpdater segmentUpdaterMock
	segmentUpdater.On("SynchronizeSegment", "segment1", (*int64)(nil)).Return(&segment.UpdateResult{}, nil).Once()

	var splitStorage splitStorageMock

	var cacheFlusher cacheFlusherMock

	var segmentStorage segmentStorageMock
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
	var segmentUpdater segmentUpdaterMock
	segmentUpdater.On("SynchronizeSegment", "segment1", (*int64)(nil)).Return(&segment.UpdateResult{
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: 2,
	}, nil).Once()

	var splitStorage splitStorageMock

	var cacheFlusher cacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MakeSurrogateForSegmentChanges("segment1")).Times(2)
	cacheFlusher.On("Evict", "/api/mySegments/k1").Times(2)
	cacheFlusher.On("Evict", "gzip::/api/mySegments/k1").Times(2)

	var segmentStorage segmentStorageMock
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
	var segmentUpdater segmentUpdaterMock
	segmentUpdater.On("SynchronizeSegments").Return(map[string]segment.UpdateResult{"segment2": {
		UpdatedKeys:     []string{"k1"},
		NewChangeNumber: 1,
	}}, nil).Once()

	var splitStorage splitStorageMock
	splitStorage.On("SegmentNames").Return(set.NewSet("segment2")).Once()

	var cacheFlusher cacheFlusherMock
	cacheFlusher.On("EvictBySurrogate", MakeSurrogateForSegmentChanges("segment2")).Times(1)
	cacheFlusher.On("Evict", "/api/mySegments/k1").Times(3)
	cacheFlusher.On("Evict", "gzip::/api/mySegments/k1").Times(3)

	var segmentStorage segmentStorageMock
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

// Borrowed mocks: These sohuld be in go-split-commons. but we need to wait until testify is adopted there

type splitUpdaterMock struct {
	mock.Mock
}

// LocalKill implements split.Updater
func (s *splitUpdaterMock) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	s.Called(splitName, defaultTreatment, changeNumber)
}

// SynchronizeFeatureFlags implements split.Updater
func (s *splitUpdaterMock) SynchronizeFeatureFlags(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
	args := s.Called(ffChange)
	return args.Get(0).(*split.UpdateResult), args.Error(1)
}

// SynchronizeSplits implements split.Updater
func (s *splitUpdaterMock) SynchronizeSplits(till *int64) (*split.UpdateResult, error) {
	args := s.Called(till)
	return args.Get(0).(*split.UpdateResult), args.Error(1)
}

// ----

type cacheFlusherMock struct {
	mock.Mock
}

func (c *cacheFlusherMock) Evict(key string)                  { c.Called(key) }
func (c *cacheFlusherMock) EvictAll()                         { c.Called() }
func (c *cacheFlusherMock) EvictBySurrogate(surrogate string) { c.Called(surrogate) }

// ---

type splitStorageMock struct {
	mock.Mock
}

func (s *splitStorageMock) All() []dtos.SplitDTO { panic("unimplemented") }
func (s *splitStorageMock) ChangeNumber() (int64, error) {
	args := s.Called()
	return args.Get(0).(int64), args.Error(1)
}

func (*splitStorageMock) FetchMany(splitNames []string) map[string]*dtos.SplitDTO {
	panic("unimplemented")
}
func (*splitStorageMock) GetNamesByFlagSets(sets []string) map[string][]string {
	panic("unimplemented")
}
func (*splitStorageMock) GetAllFlagSetNames() []string {
	panic("unimplemented")
}
func (*splitStorageMock) KillLocally(splitName string, defaultTreatment string, changeNumber int64) {
	panic("unimplemented")
}
func (s *splitStorageMock) SegmentNames() *set.ThreadUnsafeSet {
	return s.Called().Get(0).(*set.ThreadUnsafeSet)
}
func (s *splitStorageMock) SetChangeNumber(changeNumber int64) error {
	return s.Called(changeNumber).Error(0)
}
func (*splitStorageMock) Split(splitName string) *dtos.SplitDTO     { panic("unimplemented") }
func (*splitStorageMock) SplitNames() []string                      { panic("unimplemented") }
func (*splitStorageMock) TrafficTypeExists(trafficType string) bool { panic("unimplemented") }
func (*splitStorageMock) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) {
	panic("unimplemented")
}
func (*splitStorageMock) GetAllFlagSetNames() []string { return make([]string, 0) }

type segmentUpdaterMock struct {
	mock.Mock
}

func (s *segmentUpdaterMock) IsSegmentCached(segmentName string) bool { panic("unimplemented") }
func (s *segmentUpdaterMock) SegmentNames() []interface{}             { panic("unimplemented") }

func (s *segmentUpdaterMock) SynchronizeSegment(name string, till *int64) (*segment.UpdateResult, error) {
	args := s.Called(name, till)
	return args.Get(0).(*segment.UpdateResult), args.Error(1)
}

func (s *segmentUpdaterMock) SynchronizeSegments() (map[string]segment.UpdateResult, error) {
	args := s.Called()
	return args.Get(0).(map[string]segment.UpdateResult), args.Error(1)
}

type segmentStorageMock struct {
	mock.Mock
}

func (*segmentStorageMock) SetChangeNumber(segmentName string, till int64) error {
	panic("unimplemented")
}
func (s *segmentStorageMock) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, changeNumber int64) error {
	return s.Called(name, toAdd, toRemove, changeNumber).Error(0)
}

// ChangeNumber implements storage.SegmentStorage
func (s *segmentStorageMock) ChangeNumber(segmentName string) (int64, error) {
	args := s.Called(segmentName)
	return args.Get(0).(int64), args.Error(1)
}

func (*segmentStorageMock) Keys(segmentName string) *set.ThreadUnsafeSet { panic("unimplemented") }
func (*segmentStorageMock) SegmentContainsKey(segmentName string, key string) (bool, error) {
	panic("unimplemented")
}
func (*segmentStorageMock) SegmentKeysCount() int64 { panic("unimplemented") }

/*
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
*/
var _ split.Updater = (*splitUpdaterMock)(nil)
var _ storage.SplitStorage = (*splitStorageMock)(nil)
var _ gincache.CacheFlusher = (*cacheFlusherMock)(nil)
var _ segment.Updater = (*segmentUpdaterMock)(nil)
var _ storage.SegmentStorage = (*segmentStorageMock)(nil)
