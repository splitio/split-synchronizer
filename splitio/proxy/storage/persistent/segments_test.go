package persistent

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestSegmentPersistentStorage(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	segmentC := NewSegmentChangesCollection(dbw, logger)
	segmentC.Update("s1", set.NewSet("k1", "k2"), set.NewSet(), 1)
	forS1, err := segmentC.Fetch("s1")
	if err != nil {
		t.Error("err shoud be nil: ", err)
	}

	if forS1.Name != "s1" {
		t.Error("name should be `s1`")
	}

	if len(forS1.Keys) != 2 {
		t.Error("should have 2 keys")
	}

	if forS1.Keys["k1"].Removed {
		t.Error("k1 should not be removed")
	}

	forS2, err := segmentC.Fetch("s2")
	if forS2 != nil {
		t.Error("s2 should not yet exist.", forS2, err)
	}

	segmentC.Update("s1", set.NewSet(), set.NewSet("k1"), 2)
	forS1, err = segmentC.Fetch("s1")
	if err != nil {
		t.Error("err shoud be nil: ", err)
	}

	if forS1.Name != "s1" {
		t.Error("name should be `s1`")
	}

	if len(forS1.Keys) != 2 {
		t.Error("should have 2 keys", forS1)
	}

	if !forS1.Keys["k1"].Removed {
		t.Error("k1 should be removed")
	}
}
