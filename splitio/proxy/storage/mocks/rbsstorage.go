package mocks

import (
	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/stretchr/testify/mock"
)

type MockProxyRuleBasedSegmentStorage struct {
	mock.Mock
}

// ChangeNumber mock
func (m *MockProxyRuleBasedSegmentStorage) ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error) {
	args := m.Called(since)
	var dto *dtos.RuleBasedSegmentsDTO
	if args.Get(0) != nil {
		dto = args.Get(0).(*dtos.RuleBasedSegmentsDTO)
	}
	if len(args) < 2 {
		return dto, nil
	}
	return dto, args.Error(1)
}

var _ storage.ProxyRuleBasedSegmentsStorage = (*MockProxyRuleBasedSegmentStorage)(nil)
