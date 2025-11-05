package controllers

import (
	"testing"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/storage/mocks"
	"github.com/splitio/split-synchronizer/v5/splitio/admin/views/dashboard"
	"github.com/stretchr/testify/assert"
)

func TestBundleRBInfo(t *testing.T) {
	rb := &mocks.MockRuleBasedSegmentStorage{}
	rb.On("All").Return([]dtos.RuleBasedSegmentDTO{
		{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE", Excluded: dtos.ExcludedDTO{Keys: []string{"one"}}},
		{Name: "rb2", ChangeNumber: 2, Status: "ARCHIVED"},
	}, nil)
	result := bundleRBInfo(rb)
	assert.Len(t, result, 2)
	assert.ElementsMatch(t, result, []dashboard.RBSummary{
		{Name: "rb1", ChangeNumber: 1, Active: true, ExcludedKeys: []string{"one"}, ExcludedSegments: []string{}},
		{Name: "rb2", ChangeNumber: 2, Active: false, ExcludedKeys: nil, ExcludedSegments: []string{}},
	})
}
