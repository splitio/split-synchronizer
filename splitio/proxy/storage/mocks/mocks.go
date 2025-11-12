package mocks

import (
	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/stretchr/testify/mock"
)

type ProxySplitStorageMock struct {
	mock.Mock
}

func (p *ProxySplitStorageMock) ChangesSince(since int64, sets []string) (*dtos.SplitChangesDTO, error) {
	args := p.Called(since, sets)
	return args.Get(0).(*dtos.SplitChangesDTO), args.Error(1)
}

func (p *ProxySplitStorageMock) RegisterOlderCn(payload *dtos.SplitChangesDTO) {
	p.Called(payload)
}

type ProxySegmentStorageMock struct {
	mock.Mock
}

func (p *ProxySegmentStorageMock) ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error) {
	args := p.Called(name, since)
	return args.Get(0).(*dtos.SegmentChangesDTO), args.Error(1)
}

func (p *ProxySegmentStorageMock) SegmentsFor(key string) ([]string, error) {
	args := p.Called(key)
	return args.Get(0).([]string), args.Error(1)
}

func (p *ProxySegmentStorageMock) CountRemovedKeys(segmentName string) int {
	return p.Called(segmentName).Int(0)
}

type ProxyLargeSegmentStorageMock struct {
	mock.Mock
}

func (s *ProxyLargeSegmentStorageMock) SetChangeNumber(name string, till int64) {
	s.Called(name, till).Error(0)
}

func (s *ProxyLargeSegmentStorageMock) Update(name string, userKeys []string, till int64) {
	s.Called(name, userKeys, till)
}

func (s *ProxyLargeSegmentStorageMock) ChangeNumber(name string) int64 {
	args := s.Called(name)
	return args.Get(0).(int64)
}

func (s *ProxyLargeSegmentStorageMock) Count() int {
	args := s.Called()
	return args.Get(0).(int)
}

func (s *ProxyLargeSegmentStorageMock) LargeSegmentsForUser(userKey string) []string {
	args := s.Called(userKey)
	return args.Get(0).([]string)
}

func (s *ProxyLargeSegmentStorageMock) IsInLargeSegment(name string, key string) (bool, error) {
	args := s.Called(name, key)
	return args.Get(0).(bool), args.Error(1)
}

func (s *ProxyLargeSegmentStorageMock) TotalKeys(name string) int {
	return s.Called(name).Get(0).(int)
}
