package persistent

import (
	"testing"

	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestSplitPersistentStorage(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	splitC := NewSplitChangesCollection(dbw, logger)

	splitC.Update([]dtos.SplitDTO{
		{Name: "s1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "s2", ChangeNumber: 1, Status: "ACTIVE"},
	}, nil, 1)

	all, err := splitC.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "s1" || all[1].Name != "s2" {
		t.Error("Invalid payload in fetched changes.")
	}

	if splitC.ChangeNumber() != 1 {
		t.Error("CN should be 1.")
	}

	splitC.Update([]dtos.SplitDTO{{Name: "s1", ChangeNumber: 2, Status: "ARCHIVED"}}, nil, 2)
	all, err = splitC.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "s1" || all[0].Status != "ARCHIVED" {
		t.Error("s1 should be archived.")
	}

	if splitC.ChangeNumber() != 2 {
		t.Error("CN should be 2.")
	}
}
