package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestRBFromDisk(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	rbs := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs2", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
	}

	disk := persistent.NewRBChangesCollection(dbw, logger)
	disk.Update(rbs, nil, 10)
	rbsStorage := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)
	assert.ElementsMatch(t, rbs, rbsStorage.All())
}

func TestRBStorage(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	rbsStorage := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)

	rbs := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs2", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	rbsStorage.Update(rbs, nil, 10)

	assert.ElementsMatch(t, rbs, rbsStorage.All())
	cn, _ := rbsStorage.ChangeNumber()
	assert.Equal(t, int64(10), cn)
	assert.False(t, rbsStorage.Contains([]string{"rbs1", "rbs2", "rbs3"}))
	assert.True(t, rbsStorage.Contains([]string{"rbs1", "rbs2"}))

	fetchMany := rbsStorage.FetchMany([]string{"rbs1", "rbs2", "rbs3"})
	assert.Equal(t, 3, len(fetchMany))
	assert.Equal(t, "rbs1", fetchMany["rbs1"].Name)
	assert.Equal(t, "rbs2", fetchMany["rbs2"].Name)
	assert.Nil(t, fetchMany["rbs3"])

	rbs1, _ := rbsStorage.GetRuleBasedSegmentByName("rbs1")
	assert.Equal(t, rbs[0], *rbs1)
	rbs2, _ := rbsStorage.GetRuleBasedSegmentByName("rbs2")
	assert.Equal(t, rbs[1], *rbs2)
	rbs3, _ := rbsStorage.GetRuleBasedSegmentByName("rbs3")
	assert.Nil(t, rbs3)
	assert.Equal(t, set.NewSet(), rbsStorage.LargeSegments())

	newRBS := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs3", ChangeNumber: 15, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs4", ChangeNumber: 15, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	assert.Nil(t, rbsStorage.ReplaceAll(newRBS, 15))
	names, _ := rbsStorage.RuleBasedSegmentNames()
	assert.ElementsMatch(t, []string{"rbs3", "rbs4"}, names)
	assert.Equal(t, set.NewSet(), rbsStorage.Segments())

	rbsStorage.SetChangeNumber(20)
	newCN, _ := rbsStorage.ChangeNumber()
	assert.Equal(t, int64(20), newCN)

	rbsToAdd := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs5", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	rbsToRemove := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs3", ChangeNumber: 25, Status: "ARCHIVED"},
	}
	assert.Nil(t, rbsStorage.Update(rbsToAdd, rbsToRemove, 25))
	allRBS := rbsStorage.All()
	expectedRBS := []dtos.RuleBasedSegmentDTO{
		{Name: "rbs4", ChangeNumber: 15, Status: "ACTIVE", TrafficTypeName: "user"},
		{Name: "rbs5", ChangeNumber: 25, Status: "ACTIVE", TrafficTypeName: "user"},
	}
	assert.ElementsMatch(t, expectedRBS, allRBS)
}

func TestRBSChangesSince(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	pss := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)

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

func TestRBSnapshotFromDiskWithBucketNotFoundError(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	rbsStorage := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)

	assert.Empty(t, rbsStorage.All())
	assert.Equal(t, int64(-1), rbsStorage.oldestKnownCN)
}

func TestRBSnapshotFromDiskWithOtherError(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	disk := persistent.NewRBChangesCollection(dbw, logger)
	disk.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rbs1", ChangeNumber: 10, Status: "ACTIVE", TrafficTypeName: "user"},
	}, nil, 10)

	rbsStorage := NewProxyRuleBasedSegmentsStorage(dbw, logger, true)

	assert.NotEmpty(t, rbsStorage.All())
	assert.Equal(t, int64(10), rbsStorage.oldestKnownCN)
}

type recordingRBWriter struct {
	calls    int
	toAdd    []dtos.RuleBasedSegmentDTO
	toRemove []dtos.RuleBasedSegmentDTO
	cn       int64
}

func (r *recordingRBWriter) PutRuleBasedSegments(toAdd, toRemove []dtos.RuleBasedSegmentDTO, cn int64) {
	r.calls++
	r.toAdd = append(r.toAdd, toAdd...)
	r.toRemove = append(r.toRemove, toRemove...)
	r.cn = cn
}

func TestRBSStorageForwardsToRedisWriter(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logging.NewLogger(nil), false)

	w := &recordingRBWriter{}
	rbs.WithRedisWriter(w)

	toAdd := []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}}
	toRemove := []dtos.RuleBasedSegmentDTO{{Name: "rb2", ChangeNumber: 5, Status: "ARCHIVED"}}
	assert.Nil(t, rbs.Update(toAdd, toRemove, 5))

	assert.Equal(t, 1, w.calls)
	assert.Equal(t, toAdd, w.toAdd)
	assert.Equal(t, toRemove, w.toRemove)
	assert.Equal(t, int64(5), w.cn)
}

func TestRBSStorageEmptyUpdateDoesNotForward(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logging.NewLogger(nil), false)

	w := &recordingRBWriter{}
	rbs.WithRedisWriter(w)
	assert.Nil(t, rbs.Update(nil, nil, 6))

	assert.Equal(t, 0, w.calls)
}

func TestRBSStorageWithoutWriterUnchanged(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logging.NewLogger(nil), false)

	assert.Nil(t, rbs.Update([]dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 7, Status: "ACTIVE"}}, nil, 7))

	assert.Nil(t, rbs.redis)
	cn, err := rbs.ChangeNumber()
	assert.Nil(t, err)
	assert.Equal(t, int64(7), cn)
	names, err := rbs.RuleBasedSegmentNames()
	assert.Nil(t, err)
	assert.Equal(t, []string{"rb1"}, names)
}

// The writer forward does not depend on the local store write; a failing local store does not stop the change from reaching the writer
func TestRBSStorageRedisIndependentOfLocalStoreFailure(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)
	logger := logging.NewLogger(nil)
	rbs := NewProxyRuleBasedSegmentsStorage(dbw, logger, false)
	rbs.db = persistent.NewRBChangesCollection(boltDownDBWrapper{dbw}, logger)

	w := &recordingRBWriter{}
	rbs.WithRedisWriter(w)

	toAdd := []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5, Status: "ACTIVE"}}
	assert.Nil(t, rbs.Update(toAdd, nil, 5))

	assert.Equal(t, 1, w.calls, "writer receives the change although the local store fails")
	assert.Equal(t, toAdd, w.toAdd)
	cn, err := rbs.ChangeNumber()
	assert.Nil(t, err)
	assert.Equal(t, int64(5), cn, "the in-memory snapshot is updated as before")
}
