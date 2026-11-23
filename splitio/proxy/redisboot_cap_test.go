package proxy

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The till caps set while restoring stay until the first sync succeeds, and liftRestoreTillCaps lifts them.
func TestLiftRestoreTillCapsServesRealTill(t *testing.T) {
	db, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	require.NoError(t, err)
	logger := logging.NewLogger(nil)
	splits := storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := storage.NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segs := storage.NewProxySegmentStorage(db, logger, false)

	splits.Update([]dtos.SplitDTO{{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "t"}}, nil, 3)
	splits.Update([]dtos.SplitDTO{{Name: "f2", ChangeNumber: 5, Status: "ACTIVE", TrafficTypeName: "t"}}, nil, 5)
	splits.CapServedTill(3)

	changes, err := splits.ChangesSince(3, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(3), changes.Till, "capped before the first sync")

	liftRestoreTillCaps(splits, rbs, segs)

	changes, err = splits.ChangesSince(3, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(5), changes.Till, "lifted after the first sync")
}
