package mocks

import (
	"github.com/splitio/go-split-commons/v5/dtos"
)

type ProxySplitStorageMock struct {
	ChangesSinceCall    func(since int64) (*dtos.SplitChangesDTO, error)
	RegisterOlderCnCall func(payload *dtos.SplitChangesDTO)
}

func (p *ProxySplitStorageMock) ChangesSince(since int64) (*dtos.SplitChangesDTO, error) {
	return p.ChangesSinceCall(since)
}

func (p *ProxySplitStorageMock) RegisterOlderCn(payload *dtos.SplitChangesDTO) {
	p.RegisterOlderCnCall(payload)
}

type ProxySegmentStorageMock struct {
	ChangesSinceCall     func(name string, since int64) (*dtos.SegmentChangesDTO, error)
	SegmentsForCall      func(key string) ([]string, error)
	CountRemovedKeysCall func(segmentName string) int
}

func (p *ProxySegmentStorageMock) ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error) {
	return p.ChangesSinceCall(name, since)
}

func (p *ProxySegmentStorageMock) SegmentsFor(key string) ([]string, error) {
	return p.SegmentsForCall(key)
}

func (p *ProxySegmentStorageMock) CountRemovedKeys(segmentName string) int {
	return p.CountRemovedKeysCall(segmentName)
}
