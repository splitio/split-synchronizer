package optimized

import (
	"testing"

	"github.com/splitio/go-split-commons/v9/dtos"

	"github.com/stretchr/testify/assert"
)

func TestHistoricRuleBasedSegmentStorage(t *testing.T) {
	var historic HistoricChangesRBImpl
	historic.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", Status: "ACTIVE", ChangeNumber: 1, TrafficTypeName: "tt1"},
	}, []dtos.RuleBasedSegmentDTO{}, 1)
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 1},
		},
		historic.GetUpdatedSince(-1))

	// process an update with no change in rule-based segments status
	// - fetching from -1 && 1 should return the same paylaod as before with only `lastUpdated` bumped to 2
	// - fetching from 2 should return empty
	historic.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", Status: "ACTIVE", ChangeNumber: 2, TrafficTypeName: "tt1"},
	}, []dtos.RuleBasedSegmentDTO{}, 1)

	// no filter
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(1))
	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(2))

	// -------------------

	// process an update with one extra rule-based
	// - fetching from -1, & 1 should return the same payload
	// - fetching from 2 shuold only return rbs2
	// - fetching from 3 should return empty
	historic.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs2", Status: "ACTIVE", ChangeNumber: 3, TrafficTypeName: "tt1"},
	}, []dtos.RuleBasedSegmentDTO{}, 1)

	// assert correct behaviours for CN == 1..3 and no flag sets filter
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
			{Name: "rbs2", Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(-1))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
			{Name: "rbs2", Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(1))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs2", Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(2))
	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(3))

	// process an update that removes rbs2 (archives the rule-based)
	// fetching from -1 should not return rbs2
	// fetching from any intermediate CN should return rbs2 as archived
	// fetching from cn=5 should return empty
	historic.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs2", Status: "ARCHIVED", ChangeNumber: 5, TrafficTypeName: "tt1"},
	}, []dtos.RuleBasedSegmentDTO{}, 1)

	// without filter
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs1", Active: true, LastUpdated: 2},
			{Name: "rbs2", Active: false, LastUpdated: 5},
		},
		historic.GetUpdatedSince(1))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs2", Active: false, LastUpdated: 5},
		},
		historic.GetUpdatedSince(2))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs2", Active: false, LastUpdated: 5},
		},
		historic.GetUpdatedSince(3))
	assert.Equal(t,
		[]RBView{
			{Name: "rbs2", Active: false, LastUpdated: 5},
		},
		historic.GetUpdatedSince(4))
	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(5))

}
