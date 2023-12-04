package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized/mocks"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestSplitStorage(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	logger := logging.NewLogger(nil)

	toAdd := []dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", TrafficTypeName: "ttt"},
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", TrafficTypeName: "ttt"},
	}
	toAdd2 := []dtos.SplitDTO{{Name: "f3", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"}}
	toRemove := []dtos.SplitDTO{
		archivedDTOForView(&optimized.FeatureView{Name: "f2", Active: false, LastUpdated: 4, TrafficTypeName: "ttt"}),
	}

	splitC := persistent.NewSplitChangesCollection(dbw, logger)
	splitC.Update(toAdd, nil, 2)

	var historicMock mocks.HistoricStorageMock
	historicMock.On("Update", toAdd2, []dtos.SplitDTO(nil), int64(3)).Once()
	historicMock.On("GetUpdatedSince", int64(2), []string(nil)).Once().Return([]optimized.FeatureView{})

	pss := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), true)

	// validate initial state of the historic cache & replace it with a mock for the next validations
	assert.ElementsMatch(t,
		[]optimized.FeatureView{
			{Name: "f1", Active: true, LastUpdated: 1, FlagSets: []optimized.FlagSetView{}, TrafficTypeName: "ttt"},
			{Name: "f2", Active: true, LastUpdated: 2, FlagSets: []optimized.FlagSetView{}, TrafficTypeName: "ttt"},
		}, pss.historic.GetUpdatedSince(-1, nil))
	pss.historic = &historicMock
	// ----

	changes, err := pss.ChangesSince(-1, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(2), changes.Till)
	assert.ElementsMatch(t, changes.Splits, toAdd)

	changes, err = pss.ChangesSince(2, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(2), changes.Since)
	assert.Equal(t, int64(2), changes.Till)
	assert.Empty(t, changes.Splits)

	pss.Update(toAdd2, nil, 3)
	historicMock.On("GetUpdatedSince", int64(2), []string(nil)).
		Once().
		Return([]optimized.FeatureView{{Name: "f3", LastUpdated: 3, Active: true, TrafficTypeName: "ttt"}})

	changes, err = pss.ChangesSince(-1, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(3), changes.Till)
	assert.ElementsMatch(t, changes.Splits, append(append([]dtos.SplitDTO(nil), toAdd...), toAdd2...))

	changes, err = pss.ChangesSince(2, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(2), changes.Since)
	assert.Equal(t, int64(3), changes.Till)
	assert.ElementsMatch(t, changes.Splits, toAdd2)

	// archive split2 and check it's no longer returned
	historicMock.On("Update", []dtos.SplitDTO(nil), toRemove, int64(4)).Once()
	pss.Update(nil, toRemove, 4)
	historicMock.On("GetUpdatedSince", int64(3), []string(nil)).
		Once().
		Return([]optimized.FeatureView{{Name: "f2", LastUpdated: 4, Active: false, TrafficTypeName: "ttt"}})

	changes, err = pss.ChangesSince(-1, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(4), changes.Till)
	assert.ElementsMatch(t,
		[]dtos.SplitDTO{
			{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", TrafficTypeName: "ttt"},
			{Name: "f3", ChangeNumber: 3, Status: "ACTIVE", TrafficTypeName: "ttt"},
		},
		changes.Splits)

	changes, err = pss.ChangesSince(3, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(3), changes.Since)
	assert.Equal(t, int64(4), changes.Till)
	assert.ElementsMatch(t, toRemove, changes.Splits)

	historicMock.AssertExpectations(t)
}

func TestSplitStorageWithFlagsets(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)

	pss := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), true)

	pss.Update([]dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", Sets: []string{"s1", "s2"}},
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, nil, 2)

	res, err := pss.ChangesSince(-1, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(2), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", Sets: []string{"s1", "s2"}},
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, res.Splits)

	// check for s1
	res, err = pss.ChangesSince(-1, []string{"s1"})
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(1), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", Sets: []string{"s1", "s2"}},
	}, res.Splits)

	// check for s2
	res, err = pss.ChangesSince(-1, []string{"s2"})
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(2), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE", Sets: []string{"s1", "s2"}},
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, res.Splits)

	// check for s3
	res, err = pss.ChangesSince(-1, []string{"s3"})
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(2), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, res.Splits)

	// ---------------------------

	// remove f1 from s2
	pss.Update([]dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 3, Status: "ACTIVE", Sets: []string{"s1"}},
	}, nil, 2)

	// fetching from -1 only returns f1
	res, err = pss.ChangesSince(-1, []string{"s2"})
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(2), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, res.Splits)

	// fetching from -1 only returns f1
	res, err = pss.ChangesSince(-1, []string{"s2"})
	assert.Nil(t, err)
	assert.Equal(t, int64(-1), res.Since)
	assert.Equal(t, int64(2), res.Till)
	assert.ElementsMatch(t, []dtos.SplitDTO{
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE", Sets: []string{"s2", "s3"}},
	}, res.Splits)
}
