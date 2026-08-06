package mocks

import (
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/stretchr/testify/mock"
)

type SegmentChangesCollectionMock struct {
	mock.Mock
}

func (s *SegmentChangesCollectionMock) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, cn int64) error {
	return s.Called(name, toAdd, toRemove, cn).Error(0)
}

func (s *SegmentChangesCollectionMock) Fetch(name string) (*persistent.SegmentChangesItem, error) {
	args := s.Called(name)
	return args.Get(0).(*persistent.SegmentChangesItem), args.Error(1)
}

func (s *SegmentChangesCollectionMock) ChangeNumber(segment string) int64 {
	return s.Called(segment).Get(0).(int64)
}

func (s *SegmentChangesCollectionMock) SetChangeNumber(segment string, cn int64) {
	s.Called(segment, cn)
}
