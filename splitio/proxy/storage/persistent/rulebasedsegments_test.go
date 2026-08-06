package persistent

import (
	"testing"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestRBChangesCollection(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	rbChangesCollection := NewRBChangesCollection(dbw, logger)

	rbChangesCollection.Update([]dtos.RuleBasedSegmentDTO{
		{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "rb2", ChangeNumber: 1, Status: "ACTIVE"},
	}, nil, 1)

	all, err := rbChangesCollection.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "rb1" || all[1].Name != "rb2" {
		t.Error("Invalid payload in fetched changes.")
	}

	if rbChangesCollection.ChangeNumber() != 1 {
		t.Error("CN should be 1.")
	}

	rbChangesCollection.Update([]dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 2, Status: "ARCHIVED"}}, nil, 2)
	all, err = rbChangesCollection.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "rb1" || all[0].Status != "ARCHIVED" {
		t.Error("rb1 should be archived.")
	}

	if rbChangesCollection.ChangeNumber() != 2 {
		t.Error("CN should be 2.")
	}
}
