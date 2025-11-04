package storage

import (
	"testing"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

func TestRBSChangesSince(t *testing.T) {
	logger := logging.NewLogger(nil)

	// Initialize storage with some test data
	pss := NewProxyRuleBasedSegmentsStorage(logger)

	// Test case 1: since == -1
	{
		initialRuleBaseds := []dtos.RuleBasedSegmentDTO{
			{Name: "rbs1", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
			{Name: "rbs2", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
		}
		pss.Update(initialRuleBaseds, nil, 10)

		changes, err := pss.ChangesSince(-1)
		assert.Nil(t, err)
		assert.Equal(t, int64(-1), changes.Since)
		assert.Equal(t, int64(10), changes.Till)
		assert.ElementsMatch(t, initialRuleBaseds, changes.RuleBasedSegments)
	}

	// Test case 2: Error when since is too old
	{
		// The storage was initialized with CN 10, so requesting CN 5 should fail
		changes, err := pss.ChangesSince(5)
		assert.Equal(t, ErrSinceParamTooOld, err)
		assert.Nil(t, changes)
	}

	// Test case 3: Active and archived rule-based segment
	{
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

		changes, err := pss.ChangesSince(10)
		assert.Nil(t, err)
		assert.Equal(t, int64(10), changes.Since)
		assert.Equal(t, int64(15), changes.Till)

		// Should include both the new active rule-based segment and the archived one
		expectedRBSs := []dtos.RuleBasedSegmentDTO{
			{
				Name:         "rbs2",
				ChangeNumber: 15,
				Status:      "ARCHIVED",
				// Note: Archived segments have minimal fields set by archivedRBDTOForView
			},
			{
				Name:            "rbs3",
				ChangeNumber:    15,
				Status:          "ACTIVE",
				TrafficTypeName: "user",
				Excluded: dtos.ExcludedDTO{
					Keys:     nil,
					Segments: nil,
				},
				Conditions: nil,
			},
		}
		assert.ElementsMatch(t, expectedRBSs, changes.RuleBasedSegments)
	}

	// Test case 4: Proper till calculation with multiple changes
	{
		// Add changes with different change numbers
		changes1 := []dtos.RuleBasedSegmentDTO{{Name: "rbs6", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"}}
		changes2 := []dtos.RuleBasedSegmentDTO{{Name: "rbs7", ChangeNumber: 30, Status: "ACTIVE", TrafficTypeName: "user"}}

		pss.Update(changes1, nil, 25)
		pss.Update(changes2, nil, 30)

		changes, err := pss.ChangesSince(20)
		assert.Nil(t, err)
		assert.Equal(t, int64(20), changes.Since)
		assert.Equal(t, int64(30), changes.Till)
		expectedChanges := []dtos.RuleBasedSegmentDTO{
			{Name: "rbs6", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"},
			{Name: "rbs7", ChangeNumber: 30, Status: "ACTIVE", TrafficTypeName: "user"},
		}
		assert.ElementsMatch(t, expectedChanges, changes.RuleBasedSegments)
	}
}
