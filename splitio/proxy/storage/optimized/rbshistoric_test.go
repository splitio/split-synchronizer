// package optimized

// import (
// 	"testing"

// 	"github.com/splitio/go-split-commons/v8/dtos"

// 	"github.com/stretchr/testify/assert"
// )

// func TestHistoricRuleBasedSegmentStorage(t *testing.T) {
// 	var historic HistoricChangesRBImpl
// 	historic.Update([]dtos.RuleBasedSegmentDTO{
// 		{Name: "f1", Status: "ACTIVE", ChangeNumber: 1, TrafficTypeName: "tt1"},
// 	}, []dtos.RuleBasedSegmentDTO{}, 1)
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 1},
// 		},
// 		historic.GetUpdatedSince(-1))

// 	// process an update with no change in flagsets / rule-based segment status
// 	// - fetching from -1 && 1 should return the same paylaod as before with only `lastUpdated` bumped to 2
// 	// - fetching from 2 should return empty
// 	historic.Update([]dtos.RuleBasedSegmentDTO{
// 		{Name: "f1", Status: "ACTIVE", ChangeNumber: 2, TrafficTypeName: "tt1"},
// 	}, []dtos.RuleBasedSegmentDTO{}, 1)

// 	// no filter
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 		},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(2))

// 	// -------------------

// 	// process an update with one extra split
// 	// - fetching from -1, & 1 should return the same payload
// 	// - fetching from 2 shuold only return f2
// 	// - fetching from 3 should return empty
// 	historic.Update([]dtos.RuleBasedSegmentDTO{
// 		{Name: "f2", Status: "ACTIVE", ChangeNumber: 3, TrafficTypeName: "tt1"},
// 	}, []dtos.RuleBasedSegmentDTO{}, 1)

// 	// assert correct behaviours for CN == 1..3 and no flag sets filter
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(2))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(3))

// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(2))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(3))

// 	// filtering by s2:
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 2},
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(2))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(3))

// 	//filtering by s3
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 		},
// 		historic.GetUpdatedSince(2))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(3))

// 	// -------------------

// 	// process an update that removes f1 from flagset s1
// 	// - fetching without a filter should remain the same
// 	// - fetching with filter = s1 should not return f1 in CN=-1, should return it without the flagset in greater CNs
// 	historic.Update([]dtos.RuleBasedSegmentDTO{
// 		{Name: "f1", Status: "ACTIVE", ChangeNumber: 4, TrafficTypeName: "tt1"},
// 	}, []dtos.RuleBasedSegmentDTO{}, 1)

// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: true, LastUpdated: 3},
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 		},
// 		historic.GetUpdatedSince(-1))

// 	// with filter = s1 (f2 never was associated with s1, f1 is no longer associated)
// 	assert.Equal(t,
// 		[]RBView{},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 		},
// 		historic.GetUpdatedSince(2))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 		},
// 		historic.GetUpdatedSince(3))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(4))

// 	// process an update that removes f2 (archives the feature)
// 	// fetching from -1 should not return f2
// 	// fetching from any intermediate CN should return f2 as archived
// 	// fetching from cn=5 should return empty
// 	historic.Update([]dtos.RuleBasedSegmentDTO{
// 		{Name: "f2", Status: "ARCHIVED", ChangeNumber: 5, TrafficTypeName: "tt1"},
// 	}, []dtos.RuleBasedSegmentDTO{}, 1)

// 	// without filter
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 		},
// 		historic.GetUpdatedSince(-1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 			{Name: "f2", Active: false, LastUpdated: 5},
// 		},
// 		historic.GetUpdatedSince(1))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 			{Name: "f2", Active: false, LastUpdated: 5},
// 		},
// 		historic.GetUpdatedSince(2))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f1", Active: true, LastUpdated: 4},
// 			{Name: "f2", Active: false, LastUpdated: 5},
// 		},
// 		historic.GetUpdatedSince(3))
// 	assert.Equal(t,
// 		[]RBView{
// 			{Name: "f2", Active: false, LastUpdated: 5},
// 		},
// 		historic.GetUpdatedSince(4))
// 	assert.Equal(t, []RBView{}, historic.GetUpdatedSince(5))

// }
