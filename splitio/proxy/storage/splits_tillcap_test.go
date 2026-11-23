package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestSplitsServedTillCap(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	logger := logging.NewLogger(nil)

	// seed a change at cn 2 on disk so the restored storage starts at 2 and ChangesSince(2) is not too old
	persistent.NewSplitChangesCollection(dbw, logger).Update([]dtos.SplitDTO{
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", TrafficTypeName: "ttt"},
	}, nil, 2)
	pss := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), true)

	pss.Update([]dtos.SplitDTO{{Name: "f3", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"}}, nil, 3)
	pss.Update([]dtos.SplitDTO{{Name: "f5", ChangeNumber: 5, Status: "ACTIVE", TrafficTypeName: "ttt"}}, nil, 5)

	pss.CapServedTill(4)

	changes, err := pss.ChangesSince(2, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), changes.Till)
	assert.Len(t, changes.Splits, 2)

	// since equal to the cap: never below since, and the cn 5 flag is still served
	changes, err = pss.ChangesSince(4, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), changes.Till)
	assert.Len(t, changes.Splits, 1)
	assert.Equal(t, "f5", changes.Splits[0].Name)

	pss.LiftServedTillCap()

	changes, err = pss.ChangesSince(2, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(5), changes.Till)
	assert.Len(t, changes.Splits, 2)
}
