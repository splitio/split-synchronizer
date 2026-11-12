package persistent

import (
	"errors"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
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

type saveFailCollection struct {
	CollectionWrapper
	fail   bool
	logger logging.LoggerInterface
}

func (f *saveFailCollection) SaveAs(key []byte, item interface{}) error {
	if f.fail {
		return errors.New("save failed")
	}
	return nil
}

func (f *saveFailCollection) Logger() logging.LoggerInterface { return f.logger }

func TestSplitChangesCollectionCursorAdvancesOnlyWhenSaved(t *testing.T) {
	logger := logging.NewLogger(nil)
	failing := &SplitChangesCollection{collection: &saveFailCollection{fail: true, logger: logger}}
	failing.Update([]dtos.SplitDTO{{Name: "f", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5)
	if cn := failing.ChangeNumber(); cn != 0 {
		t.Errorf("cursor must not advance when a save fails, got %d", cn)
	}

	ok := &SplitChangesCollection{collection: &saveFailCollection{logger: logger}}
	ok.Update([]dtos.SplitDTO{{Name: "f", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5)
	if cn := ok.ChangeNumber(); cn != 5 {
		t.Errorf("cursor should advance after successful saves, got %d", cn)
	}
}
