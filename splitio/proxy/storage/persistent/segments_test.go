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

func TestNewSegmentChangesCollectionInitializesChangeNumbers(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)

	// First, create a collection and add some segments with different change numbers
	segmentC := NewSegmentChangesCollection(dbw, logger)

	// Add segment s1 with keys at change number 10
	segmentC.Update("s1", set.NewSet("k1", "k2"), set.NewSet(), 10)

	// Add more keys to s1 at change number 20
	segmentC.Update("s1", set.NewSet("k3"), set.NewSet(), 20)

	// Add segment s2 with keys at change number 15
	segmentC.Update("s2", set.NewSet("k4", "k5"), set.NewSet(), 15)

	// Verify change numbers are set correctly after updates
	if cn := segmentC.ChangeNumber("s1"); cn != 20 {
		t.Errorf("s1 change number should be 20, got %d", cn)
	}

	if cn := segmentC.ChangeNumber("s2"); cn != 15 {
		t.Errorf("s2 change number should be 15, got %d", cn)
	}

	// Now create a NEW collection from the same database (simulating a restart with snapshot)
	segmentC2 := NewSegmentChangesCollection(dbw, logger)

	// Verify that the change numbers were initialized correctly from the stored data
	if cn := segmentC2.ChangeNumber("s1"); cn != 20 {
		t.Errorf("After initialization, s1 change number should be 20, got %d", cn)
	}

	if cn := segmentC2.ChangeNumber("s2"); cn != 15 {
		t.Errorf("After initialization, s2 change number should be 15, got %d", cn)
	}

	// Verify non-existent segment returns -1
	if cn := segmentC2.ChangeNumber("nonexistent"); cn != -1 {
		t.Errorf("Non-existent segment should return -1, got %d", cn)
	}
}

func TestInitializeWithEmptyDatabase(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)

	// Create a collection on an empty database
	segmentC := NewSegmentChangesCollection(dbw, logger)

	// Should not panic and should return -1 for any segment
	if cn := segmentC.ChangeNumber("any_segment"); cn != -1 {
		t.Errorf("Empty database should return -1 for any segment, got %d", cn)
	}
}

func TestInitializeWithRemovedKeys(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	segmentC := NewSegmentChangesCollection(dbw, logger)

	// Add keys at change number 10
	segmentC.Update("s1", set.NewSet("k1", "k2", "k3"), set.NewSet(), 10)

	// Remove some keys at change number 25
	segmentC.Update("s1", set.NewSet(), set.NewSet("k1"), 25)

	// Add new keys at change number 30
	segmentC.Update("s1", set.NewSet("k4"), set.NewSet(), 30)

	// The max change number should be 30
	if cn := segmentC.ChangeNumber("s1"); cn != 30 {
		t.Errorf("s1 change number should be 30, got %d", cn)
	}

	// Create new collection to test initialization
	segmentC2 := NewSegmentChangesCollection(dbw, logger)

	// Should pick up the maximum change number (30) even with mixed add/remove operations
	if cn := segmentC2.ChangeNumber("s1"); cn != 30 {
		t.Errorf("After initialization with removed keys, s1 change number should be 30, got %d", cn)
	}
}

func TestInitializeMultipleSegments(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	segmentC := NewSegmentChangesCollection(dbw, logger)

	// Create multiple segments with different change numbers
	segments := map[string]int64{
		"segment_a": 100,
		"segment_b": 250,
		"segment_c": 50,
		"segment_d": 999,
	}

	for name, cn := range segments {
		segmentC.Update(name, set.NewSet("key1", "key2"), set.NewSet(), cn)
	}

	// Create new collection and verify all segments are initialized correctly
	segmentC2 := NewSegmentChangesCollection(dbw, logger)

	for name, expectedCN := range segments {
		if cn := segmentC2.ChangeNumber(name); cn != expectedCN {
			t.Errorf("Segment %s should have change number %d, got %d", name, expectedCN, cn)
		}
	}
}
