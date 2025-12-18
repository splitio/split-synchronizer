package mocks

import (
	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/stretchr/testify/mock"
)

type MockProxyRuleBasedSegmentStorage struct {
	mock.Mock
}

// ChangeNumber mock
func (m *MockProxyRuleBasedSegmentStorage) ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error) {
	args := m.Called(since)
	return args.Get(0).(*dtos.RuleBasedSegmentsDTO), nil
}

var _ storage.ProxyRuleBasedSegmentsStorage = (*MockProxyRuleBasedSegmentStorage)(nil)
