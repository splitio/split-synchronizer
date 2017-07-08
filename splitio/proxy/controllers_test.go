package proxy

import (
	"fmt"
	"testing"
	"time"

	"github.com/splitio/go-agent/splitio/storage/boltdb"
	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"
)

func TestSplitController(t *testing.T) {

	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/test_controller_splits_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	boltdb.DBB = db

	var split1 = &collections.SplitChangesItem{Name: "SPLIT_1", ChangeNumber: 333333, Status: "ACTIVE", JSON: "some_json_split1"}
	var split2 = &collections.SplitChangesItem{Name: "SPLIT_2", ChangeNumber: 222222, Status: "KILLED", JSON: "some_json_split2"}
	var split3 = &collections.SplitChangesItem{Name: "SPLIT_3", ChangeNumber: 111111, Status: "ACTIVE", JSON: "some_json_split3"}

	splitCollection := collections.NewSplitChangesCollection(db)

	erra := splitCollection.Add(split1)
	if erra != nil {
		t.Error(erra)
	}

	erra = splitCollection.Add(split2)
	if erra != nil {
		t.Error(erra)
	}

	erra = splitCollection.Add(split3)
	if erra != nil {
		t.Error(erra)
	}

	// Since = -1
	splits, till, errf := fetchSplitsFromDB(-1)
	if errf != nil {
		t.Error(errf)
	}

	if len(splits) != 2 {
		t.Error("Invalid len result")
	}

	if till != 333333 {
		t.Error("Invalid TILL value")
	}

	//Since = 222222
	splits, till, errf = fetchSplitsFromDB(222222)
	if errf != nil {
		t.Error(errf)
	}

	if len(splits) != 1 {
		t.Error("Invalid len result")
	}

	if till != 333333 {
		t.Error("Invalid TILL value")
	}
}

func TestSegmentController(t *testing.T) {

	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/test_controller_segments_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	boltdb.DBB = db
	segmentName := "SEGMENT_TEST"

	var segment = &collections.SegmentChangesItem{Name: segmentName}
	segment.Keys = make(map[string]collections.SegmentKey)

	key1 := collections.SegmentKey{Name: "Key_1", Removed: false, ChangeNumber: 1}
	key2 := collections.SegmentKey{Name: "Key_2", Removed: false, ChangeNumber: 2}
	key3 := collections.SegmentKey{Name: "Key_3", Removed: false, ChangeNumber: 3}
	key4 := collections.SegmentKey{Name: "Key_4", Removed: true, ChangeNumber: 4}

	segment.Keys[key1.Name] = key1
	segment.Keys[key2.Name] = key2
	segment.Keys[key3.Name] = key3
	segment.Keys[key4.Name] = key4

	col := collections.NewSegmentChangesCollection(boltdb.DBB)

	// test Add
	errs := col.Add(segment)
	if errs != nil {
		t.Error(errs)
	}

	added, removed, till, errf := fetchSegmentsFromDB(-1, segmentName)
	if errf != nil {
		t.Error(errf)
	}

	if till != 3 {
		t.Error("Incorrect TILL value")
	}

	if len(added) != 3 {
		t.Error("Wrong number of keys in ADDED")
	}

	if len(removed) != 0 {
		t.Error("Wrong number of keys in REMOVED")
	}
	// test keys
	if !inSegmentArray(added, key1.Name) || !inSegmentArray(added, key2.Name) || !inSegmentArray(added, key3.Name) {
		t.Error("Missing key")
	}

	if inSegmentArray(added, key4.Name) {
		t.Error("Removed keys musn't be added")
	}

	added, removed, till, errf = fetchSegmentsFromDB(3, segmentName)
	if errf != nil {
		t.Error(errf)
	}

	if till != 4 {
		t.Error("Incorrect TILL value")
	}

	if len(added) != 0 {
		t.Error("Wrong number of keys in ADDED")
	}

	if len(removed) != 1 {
		t.Error("Wrong number of keys in REMOVED")
	}
	// testing keys
	if !inSegmentArray(removed, key4.Name) {
		t.Error("Invalid key added in REMOVED array")
	}
}

func inSegmentArray(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
