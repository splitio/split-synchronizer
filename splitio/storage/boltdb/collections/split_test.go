package collections

import (
	"fmt"
	"testing"
	"time"

	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

func TestSplitCollection(t *testing.T) {
	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/testcollection_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	var split1 = &SplitChangesItem{Name: "SPLIT_1", ChangeNumber: 999888, Status: "ACTIVE", JSON: "some_json_split1"}
	var split2 = &SplitChangesItem{Name: "SPLIT_2", ChangeNumber: 555555, Status: "KILLED", JSON: "some_json_split2"}

	splitCollection := NewSplitChangesCollection(db)

	erra := splitCollection.Add(split1)
	if erra != nil {
		t.Error(erra)
	}

	erra = splitCollection.Add(split2)
	if erra != nil {
		t.Error(erra)
	}

	items, errs := splitCollection.FetchAll()
	if errs != nil {
		t.Error(errs)
	}

	if len(items) != 2 {
		t.Error("Bad len of items")
	}

	if items[0].ChangeNumber != 999888 {
		t.Error("Incorrect order list")
	}

}

func TestSplitCollectionDeleteItem(t *testing.T) {
	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/testcollection_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	var split1 = &SplitChangesItem{Name: "SPLIT_1", ChangeNumber: 999888, Status: "ACTIVE", JSON: fmt.Sprintf("%d", time.Now().UnixNano())}

	splitCollection := NewSplitChangesCollection(db)

	for i := 1; i < 10; i++ {
		erra := splitCollection.Add(split1)
		if erra != nil {
			t.Error(erra)
		}
	}

	errd := splitCollection.Delete(split1)
	if errd != nil {
		t.Error(errd)
	}

	items, errs := splitCollection.FetchAll()
	if errs != nil {
		t.Error(errs)
	}

	if len(items) != 0 {
		t.Error("Bad len of items")
	}

}
