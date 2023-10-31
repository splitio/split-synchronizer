package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestSplitStorage(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	splitC := persistent.NewSplitChangesCollection(dbw, logger)

	splitC.Update([]dtos.SplitDTO{
		{Name: "f1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "f2", ChangeNumber: 2, Status: "ACTIVE"},
	}, nil, 1)

	pss := NewProxySplitStorage(dbw, logger, flagsets.NewFlagSetFilter(nil), true)

	sinceMinus1, currentCN, err := pss.recipes.FetchSince(-1)
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	if currentCN != 2 {
		t.Error("current cn should be 2. Is: ", currentCN)
	}

	if _, ok := sinceMinus1.Updated["f1"]; !ok {
		t.Error("s1 should be added")
	}

	if _, ok := sinceMinus1.Updated["f2"]; !ok {
		t.Error("s2 should be added")
	}

	since2, currentCN, err := pss.recipes.FetchSince(2)
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	if currentCN != 2 {
		t.Error("current cn should be 2. Is: ", currentCN)
	}

	if len(since2.Updated) != 0 {
		t.Error("nothing should have been added")
	}

	if len(since2.Removed) != 0 {
		t.Error("nothing should have been removed")
	}
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
