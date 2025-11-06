package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestRBSChangesSince(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	pss := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)

	// From -1
	rbs := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs2", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	pss.Update(rbs, nil, 10)
	changes, err := pss.ChangesSince(-1)
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(10), changes.Till)
	assert.ElementsMatch(t, rbs, changes.RuleBasedSegments)

	changes, err = pss.ChangesSince(5)
	assert.Equal(t, ErrSinceParamTooOld, err)
	assert.Nil(t, changes)

	// Add a new rule-based segment and archive an existing one
	toAdd := []dtos.RuleBasedSegmentDTO{{Name: "rbs3", ChangeNumber: 15, Status: "ACTIVE", TrafficTypeName: "user"}}
	toRemove := []dtos.RuleBasedSegmentDTO{
		{
			Name:            "rbs2",
			ChangeNumber:    15,
			Status:          "ARCHIVED",
			TrafficTypeName: "user",
			Conditions:      []dtos.RuleBasedConditionDTO{},
		},
	}
	pss.Update(toAdd, toRemove, 15)
	changes, err = pss.ChangesSince(10)
	assert.Nil(t, err)
	assert.Equal(t, int64(10), changes.Since)
	assert.Equal(t, int64(15), changes.Till)

	// Should include both the new active rule-based segment and the archived one
	expectedRBSs := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs2", ChangeNumber: 15, Status: "ARCHIVED"},
		{Name: "rbs3", ChangeNumber: 15, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	assert.ElementsMatch(t, expectedRBSs, changes.RuleBasedSegments)

	changes1 := []dtos.RuleBasedSegmentDTO{{Name: "rbs6", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"}}
	changes2 := []dtos.RuleBasedSegmentDTO{{Name: "rbs7", ChangeNumber: 30, Status: "ACTIVE", TrafficTypeName: "user"}}
	pss.Update(changes1, nil, 25)
	pss.Update(changes2, nil, 30)
	changes, err = pss.ChangesSince(20)
	assert.Nil(t, err)
	assert.Equal(t, int64(20), changes.Since)
	assert.Equal(t, int64(30), changes.Till)
	expectedChanges := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs6", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs7", ChangeNumber: 30, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	assert.ElementsMatch(t, expectedChanges, changes.RuleBasedSegments)
}
