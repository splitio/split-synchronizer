package storage

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/stretchr/testify/assert"
)

func TestSegmentsServedTillCap(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	disk := persistent.NewSegmentChangesCollection(dbw, logger)
	assert.Nil(t, disk.Update("seg", set.NewSet("k2"), set.NewSet(), 2))
	assert.Nil(t, disk.Update("seg", set.NewSet("k6"), set.NewSet(), 6))
	assert.Nil(t, disk.Update("other", set.NewSet("o6"), set.NewSet(), 6))

	ss := NewProxySegmentStorage(dbw, logger, false)

	ss.CapServedTill("seg", 4)

	dto, err := ss.ChangesSince("seg", 0)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), dto.Till)
	assert.ElementsMatch(t, []string{"k2", "k6"}, dto.Added)

	dto, err = ss.ChangesSince("seg", 4)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), dto.Till)
	assert.Equal(t, []string{"k6"}, dto.Added)

	dto, err = ss.ChangesSince("other", 0)
	assert.Nil(t, err)
	assert.Equal(t, int64(6), dto.Till)

	ss.LiftServedTillCaps()

	dto, err = ss.ChangesSince("seg", 0)
	assert.Nil(t, err)
	assert.Equal(t, int64(6), dto.Till)
}
