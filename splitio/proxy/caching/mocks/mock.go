package mocks

import (
	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/largesegment"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/datastructures/set"

	"github.com/stretchr/testify/mock"
)

// Borrowed mocks: These sohuld be in go-split-commons. but we need to wait until testify is adopted there

type SplitUpdaterMock struct {
	mock.Mock
}

// LocalKill implements split.Updater
func (s *SplitUpdaterMock) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	s.Called(splitName, defaultTreatment, changeNumber)
}

// SynchronizeFeatureFlags implements split.Updater
func (s *SplitUpdaterMock) SynchronizeFeatureFlags(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
	args := s.Called(ffChange)
	return args.Get(0).(*split.UpdateResult), args.Error(1)
}

// SynchronizeSplits implements split.Updater
func (s *SplitUpdaterMock) SynchronizeSplits(till *int64) (*split.UpdateResult, error) {
	args := s.Called(till)
	return args.Get(0).(*split.UpdateResult), args.Error(1)
}

// ----

type CacheFlusherMock struct {
	mock.Mock
}

func (c *CacheFlusherMock) Evict(key string)                  { c.Called(key) }
func (c *CacheFlusherMock) EvictAll()                         { c.Called() }
func (c *CacheFlusherMock) EvictBySurrogate(surrogate string) { c.Called(surrogate) }

// ---

type SplitStorageMock struct {
	mock.Mock
}

func (s *SplitStorageMock) All() []dtos.SplitDTO { panic("unimplemented") }
func (s *SplitStorageMock) ChangeNumber() (int64, error) {
	args := s.Called()
	return args.Get(0).(int64), args.Error(1)
}

func (*SplitStorageMock) FetchMany(splitNames []string) map[string]*dtos.SplitDTO {
	panic("unimplemented")
}

func (*SplitStorageMock) GetNamesByFlagSets(sets []string) map[string][]string {
	panic("unimplemented")
}

func (*SplitStorageMock) GetAllFlagSetNames() []string {
	panic("unimplemented")
}

func (*SplitStorageMock) KillLocally(splitName string, defaultTreatment string, changeNumber int64) {
	panic("unimplemented")
}

func (s *SplitStorageMock) SegmentNames() *set.ThreadUnsafeSet {
	return s.Called().Get(0).(*set.ThreadUnsafeSet)
}

func (s *SplitStorageMock) LargeSegmentNames() *set.ThreadUnsafeSet {
	return s.Called().Get(0).(*set.ThreadUnsafeSet)
}

func (s *SplitStorageMock) SetChangeNumber(changeNumber int64) error {
	return s.Called(changeNumber).Error(0)
}

func (*SplitStorageMock) Split(splitName string) *dtos.SplitDTO     { panic("unimplemented") }
func (*SplitStorageMock) SplitNames() []string                      { panic("unimplemented") }
func (*SplitStorageMock) TrafficTypeExists(trafficType string) bool { panic("unimplemented") }

func (*SplitStorageMock) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) {
	panic("unimplemented")
}

func (s *SplitStorageMock) ReplaceAll(splits []dtos.SplitDTO, changeNumber int64) error {
	args := s.Called(splits, changeNumber)
	return args.Error(0)
}

func (s *SplitStorageMock) RuleBasedSegmentNames() *set.ThreadUnsafeSet {
	return s.Called().Get(0).(*set.ThreadUnsafeSet)
}

// ---

type SegmentUpdaterMock struct {
	mock.Mock
}

func (s *SegmentUpdaterMock) IsSegmentCached(segmentName string) bool { panic("unimplemented") }
func (s *SegmentUpdaterMock) SegmentNames() []interface{}             { panic("unimplemented") }

func (s *SegmentUpdaterMock) SynchronizeSegment(name string, till *int64) (*segment.UpdateResult, error) {
	args := s.Called(name, till)
	return args.Get(0).(*segment.UpdateResult), args.Error(1)
}

func (s *SegmentUpdaterMock) SynchronizeSegments() (map[string]segment.UpdateResult, error) {
	args := s.Called()
	return args.Get(0).(map[string]segment.UpdateResult), args.Error(1)
}

type SegmentStorageMock struct {
	mock.Mock
}

func (*SegmentStorageMock) SetChangeNumber(segmentName string, till int64) error {
	panic("unimplemented")
}

func (s *SegmentStorageMock) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, changeNumber int64) error {
	return s.Called(name, toAdd, toRemove, changeNumber).Error(0)
}

// ChangeNumber implements storage.SegmentStorage
func (s *SegmentStorageMock) ChangeNumber(segmentName string) (int64, error) {
	args := s.Called(segmentName)
	return args.Get(0).(int64), args.Error(1)
}

func (*SegmentStorageMock) Keys(segmentName string) *set.ThreadUnsafeSet { panic("unimplemented") }

func (*SegmentStorageMock) SegmentContainsKey(segmentName string, key string) (bool, error) {
	panic("unimplemented")
}

func (*SegmentStorageMock) SegmentKeysCount() int64 { panic("unimplemented") }

// ---

type LargeSegmentStorageMock struct {
	mock.Mock
}

func (s *LargeSegmentStorageMock) SetChangeNumber(name string, till int64) {
	s.Called(name, till).Error(0)
}

func (s *LargeSegmentStorageMock) Update(name string, userKeys []string, till int64) {
	s.Called(name, userKeys, till)
}

func (s *LargeSegmentStorageMock) ChangeNumber(name string) int64 {
	args := s.Called(name)
	return args.Get(0).(int64)
}

func (s *LargeSegmentStorageMock) Count() int {
	args := s.Called()
	return args.Get(0).(int)
}

func (s *LargeSegmentStorageMock) LargeSegmentsForUser(userKey string) []string {
	return []string{}
}

func (s *LargeSegmentStorageMock) IsInLargeSegment(name string, key string) (bool, error) {
	args := s.Called(name, key)
	return args.Get(0).(bool), args.Error(1)
}

func (s *LargeSegmentStorageMock) TotalKeys(name string) int {
	return s.Called(name).Get(0).(int)
}

// ---

type LargeSegmentUpdaterMock struct {
	mock.Mock
}

func (u *LargeSegmentUpdaterMock) SynchronizeLargeSegment(name string, till *int64) (*int64, error) {
	args := u.Called(name, till)
	return args.Get(0).(*int64), args.Error(1)
}

func (u *LargeSegmentUpdaterMock) SynchronizeLargeSegments() (map[string]*int64, error) {
	args := u.Called()
	return args.Get(0).(map[string]*int64), args.Error(1)
}

func (u *LargeSegmentUpdaterMock) IsCached(name string) bool {
	return u.Called().Get(0).(bool)
}

func (u *LargeSegmentUpdaterMock) SynchronizeLargeSegmentUpdate(lsRFDResponseDTO *dtos.LargeSegmentRFDResponseDTO) (*int64, error) {
	args := u.Called(lsRFDResponseDTO)
	return args.Get(0).(*int64), args.Error(1)
}

// ---

var _ gincache.CacheFlusher = (*CacheFlusherMock)(nil)
var _ split.Updater = (*SplitUpdaterMock)(nil)
var _ segment.Updater = (*SegmentUpdaterMock)(nil)
var _ largesegment.Updater = (*LargeSegmentUpdaterMock)(nil)
var _ storage.SplitStorage = (*SplitStorageMock)(nil)
var _ storage.SegmentStorage = (*SegmentStorageMock)(nil)
var _ storage.LargeSegmentsStorage = (*LargeSegmentStorageMock)(nil)
