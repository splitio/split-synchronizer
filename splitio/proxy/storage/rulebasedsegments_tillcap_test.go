package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestRuleBasedSegmentsServedTillCap(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	logger := logging.NewLogger(nil)

	// seed a change at cn 2 on disk so the restored storage starts at 2 and ChangesSince(2) is not too old
	persistent.NewRBChangesCollection(dbw, logger).Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs2", ChangeNumber: 2, Status: "ACTIVE", TrafficTypeName: "user"},
	}, nil, 2)
	pss := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)

	pss.Update([]dtos.RuleBasedSegmentDTO{{Name: "rbs3", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "user"}}, nil, 3)
	pss.Update([]dtos.RuleBasedSegmentDTO{{Name: "rbs5", ChangeNumber: 5, Status: "ACTIVE", TrafficTypeName: "user"}}, nil, 5)

	pss.CapServedTill(4)

	changes, err := pss.ChangesSince(2)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), changes.Till)
	assert.Len(t, changes.RuleBasedSegments, 2)

	changes, err = pss.ChangesSince(4)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), changes.Till)
	assert.Len(t, changes.RuleBasedSegments, 1)
	assert.Equal(t, "rbs5", changes.RuleBasedSegments[0].Name)

	pss.LiftServedTillCap()

	changes, err = pss.ChangesSince(2)
	assert.Nil(t, err)
	assert.Equal(t, int64(5), changes.Till)
	assert.Len(t, changes.RuleBasedSegments, 2)
}
