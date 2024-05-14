package mocks

import (
	"github.com/splitio/go-split-commons/v6/dtos"
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
