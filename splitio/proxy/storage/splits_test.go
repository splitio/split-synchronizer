package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestSplitStorage(t *testing.T) {
	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	splitC := persistent.NewSplitChangesCollection(dbw, logger)

	splitC.Update([]dtos.SplitDTO{
		{Name: "s1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "s2", ChangeNumber: 2, Status: "ACTIVE"},
	}, nil, 1)

	pss := NewProxySplitStorage(dbw, logger, true)

	sinceMinus1, currentCN, err := pss.recipes.FetchSince(-1)
	if err != nil {
		t.Error("unexpected error: ", err)
	}

	if currentCN != 2 {
		t.Error("current cn should be 2. Is: ", currentCN)
	}

	if _, ok := sinceMinus1.Updated["s1"]; !ok {
		t.Error("s1 should be added")
	}

	if _, ok := sinceMinus1.Updated["s2"]; !ok {
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
