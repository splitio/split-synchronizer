package controllers

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/admin/views/dashboard"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"

	"github.com/stretchr/testify/assert"
)

func TestBundleRBInfo(t *testing.T) {
	split := &mocks.SplitStorageMock{}
	split.On("RuleBasedSegmentNames").Return(set.NewSet("rb1", "rb2"), nil).Once()
	rb := &mocks.MockRuleBasedSegmentStorage{}
	rb.On("GetRuleBasedSegmentByName", "rb1").Return(&dtos.RuleBasedSegmentDTO{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE", Excluded: dtos.ExcludedDTO{Keys: []string{"one"}}}, nil).Once()
	rb.On("GetRuleBasedSegmentByName", "rb2").Return(&dtos.RuleBasedSegmentDTO{Name: "rb2", ChangeNumber: 2, Status: "ARCHIVED"}, nil).Once()
	result := bundleRuleBasedInfo(split, rb)
	assert.Len(t, result, 2)
	assert.ElementsMatch(t, result, []dashboard.RuleBasedSegmentSummary{
		{Name: "rb1", ChangeNumber: 1, Active: true, ExcludedKeys: []string{"one"}, ExcludedSegments: []dashboard.ExcludedSegments{}, LastModified: "Thu Jan  1 00:00:00 UTC 1970"},
		{Name: "rb2", ChangeNumber: 2, Active: false, ExcludedKeys: []string{}, ExcludedSegments: []dashboard.ExcludedSegments{}, LastModified: "Thu Jan  1 00:00:00 UTC 1970"},
	})
	split.AssertExpectations(t)
	rb.AssertExpectations(t)
}
