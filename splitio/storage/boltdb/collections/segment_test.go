package collections

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

func before() {
	stdoutWriter := os.Stdout //ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	//Initialize by default
	conf.Initialize()

	conf.Data.Logger.DebugOn = true
}

func TestSegmentCollection(t *testing.T) {
	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/testsegmentcollection_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	var segment = &SegmentChangesItem{Name: "SEGMENT_1"}
	segment.Keys = make(map[string]SegmentKey)

	for j := 1; j <= 100; j++ {
		keyName := fmt.Sprintf("4d37521c-06d5-43cf-849f-4727bfdeaa0c_%d", j)
		segment.Keys[keyName] = SegmentKey{
			Name:         keyName,
			Removed:      false,
			ChangeNumber: 1498262190861}
	}

	col := NewSegmentChangesCollection(db)

	// test Add
	errs := col.Add(segment)
	if errs != nil {
		t.Error(errs)
	}

	//test Fetch
	item, erri := col.Fetch("SEGMENT_1")
	if erri != nil {
		t.Error(erri)
	}
	if item.Name != "SEGMENT_1" {
		t.Error("Invalid data fetched")
	}

	if len(item.Keys) != 100 {
		t.Error("Invalid number of keys")
	}

	for key, obj := range item.Keys {
		if key != obj.Name {
			t.Error("Key mismatch object name")
		}
	}
}

func benchmarkSegmentAllocation(t *testing.T) {
	before()
	//Test variables
	numberOfKeys := 500000
	numberOfSegments := 5
	var dbb *bolt.DB
	var err error
	//dbb, err = boltdb.NewInstance(boltdb.InMemoryMode, nil)
	dbb, err = boltdb.NewInstance("/tmp/segments.db", nil)
	if err != nil {
		t.Error(err)
		return
	}

	segmentCollection := NewSegmentChangesCollection(dbb)

	for i := 1; i <= numberOfSegments; i++ {
		segmentItem := &SegmentChangesItem{}
		segmentItem.Name = fmt.Sprintf("segment_name_%d", i)
		segmentItem.Keys = make(map[string]SegmentKey)
		keyCounter := 0
		for j := 1; j <= numberOfKeys; j++ {
			keyName := fmt.Sprintf("4d37521c-06d5-43cf-849f-4727bfdeaa0c_%d", j) //fmt.Sprintf(uuid.NewV4().String()+"_%d", j)
			segmentItem.Keys[keyName] = SegmentKey{
				Name:         keyName,
				Removed:      false,
				ChangeNumber: 1498262190861}
			keyCounter++
		}

		err = segmentCollection.Add(segmentItem)
		if err != nil {
			t.Error(err)
			return
		}
	}
}
