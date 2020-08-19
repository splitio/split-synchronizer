package mocks

import (
	"github.com/splitio/split-synchronizer/splitio/api"
)

// ImpressionObserverMock for testing purposes
type ImpressionObserverMock struct {
	TestAndSetCall func(featureName string, keyImpression *api.ImpressionDTO) (int64, error)
}

// TestAndSet call forwarding
func (m *ImpressionObserverMock) TestAndSet(featureName string, keyImpression *api.ImpressionDTO) (int64, error) {
	return m.TestAndSetCall(featureName, keyImpression)
}
