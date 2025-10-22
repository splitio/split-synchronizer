package mocks

import (
	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
	"github.com/stretchr/testify/mock"
)

type HistoricStorageMock struct {
	mock.Mock
}

// GetUpdatedSince implements optimized.HistoricChanges
func (h *HistoricStorageMock) GetUpdatedSince(since int64, flagSets []string) []optimized.FeatureView {
	return h.Called(since, flagSets).Get(0).([]optimized.FeatureView)
}

// Update implements optimized.HistoricChanges
func (h *HistoricStorageMock) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, newCN int64) {
	h.Called(toAdd, toRemove, newCN)
}

var _ optimized.HistoricChanges = (*HistoricStorageMock)(nil)
