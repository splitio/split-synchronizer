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

// TestSegmentRestoreKeepsPerKeyChangeNumbersAndTombstones checks that Restore keeps each key's
// change number and tombstones, and survives re-initialization.
func TestSegmentRestoreKeepsPerKeyChangeNumbersAndTombstones(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	segmentC := NewSegmentChangesCollection(dbw, logger)

	item := SegmentChangesItem{
		Name: "s1",
		Keys: map[string]SegmentKey{
			"k1": {Name: "k1", ChangeNumber: 5, Removed: false},
			"k2": {Name: "k2", ChangeNumber: 9, Removed: true},
			"k3": {Name: "k3", ChangeNumber: 7, Removed: false},
		},
	}
	if err := segmentC.Restore(item); err != nil {
		t.Fatal("Restore should not fail: ", err)
	}

	check := func(c *SegmentChangesCollectionImpl, label string) {
		t.Helper()
		fetched, err := c.Fetch("s1")
		if err != nil || fetched == nil {
			t.Fatalf("%s: Fetch should return s1, err: %v", label, err)
		}
		if len(fetched.Keys) != len(item.Keys) {
			t.Fatalf("%s: expected %d keys, got %d", label, len(item.Keys), len(fetched.Keys))
		}
		for name, want := range item.Keys {
			got, ok := fetched.Keys[name]
			if !ok {
				t.Errorf("%s: key %s missing", label, name)
				continue
			}
			if got.ChangeNumber != want.ChangeNumber || got.Removed != want.Removed {
				t.Errorf(
					"%s: key %s got cn=%d removed=%v, want cn=%d removed=%v",
					label,
					name,
					got.ChangeNumber,
					got.Removed,
					want.ChangeNumber,
					want.Removed,
				)
			}
		}
		if cn := c.ChangeNumber("s1"); cn != 9 {
			t.Errorf("%s: segment change number should be 9, got %d", label, cn)
		}
	}

	check(segmentC, "after restore")
	check(NewSegmentChangesCollection(dbw, logger), "after re-initialization")
}

// TestSegmentRestoreActiveKeyCountIgnoresTombstones checks that a restored segment with 1 active
// key and 3 tombstones has exactly one active key.
func TestSegmentRestoreActiveKeyCountIgnoresTombstones(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}
	segmentC := NewSegmentChangesCollection(dbw, logging.NewLogger(nil))

	item := SegmentChangesItem{
		Name: "s1",
		Keys: map[string]SegmentKey{
			"active": {Name: "active", ChangeNumber: 3},
			"t1":     {Name: "t1", ChangeNumber: 4, Removed: true},
			"t2":     {Name: "t2", ChangeNumber: 5, Removed: true},
			"t3":     {Name: "t3", ChangeNumber: 6, Removed: true},
		},
	}
	if err := segmentC.Restore(item); err != nil {
		t.Fatal("Restore should not fail: ", err)
	}

	fetched, err := segmentC.Fetch("s1")
	if err != nil || fetched == nil {
		t.Fatalf("Fetch should return s1, err: %v", err)
	}
	active := 0
	for _, key := range fetched.Keys {
		if !key.Removed {
			active++
		}
	}
	if active != 1 {
		t.Errorf("expected 1 active key, got %d", active)
	}
}

// TestSegmentRestoreWithoutKeysAnchorsChangeNumber checks that a restore with no keys still anchors
// the segment at change number zero.
func TestSegmentRestoreWithoutKeysAnchorsChangeNumber(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}
	segmentC := NewSegmentChangesCollection(dbw, logging.NewLogger(nil))

	if err := segmentC.Restore(SegmentChangesItem{Name: "empty"}); err != nil {
		t.Fatal("Restore should not fail: ", err)
	}
	if cn := segmentC.ChangeNumber("empty"); cn != 0 {
		t.Errorf("expected change number 0 for a segment without keys, got %d", cn)
	}
}

// TestSegmentRestoreReturnsBoltError checks that a failed Bolt write during Restore is returned
// and leaves the change number untouched.
func TestSegmentRestoreReturnsBoltError(t *testing.T) {
	segmentC := NewSegmentChangesCollection(closedDB(t), logging.NewLogger(nil))

	err := segmentC.Restore(SegmentChangesItem{Name: "s1", Keys: map[string]SegmentKey{"k": {Name: "k", ChangeNumber: 2}}})
	if err == nil {
		t.Fatal("expected the failed Bolt write to be returned")
	}
	if cn := segmentC.ChangeNumber("s1"); cn != -1 {
		t.Errorf("change number should not be anchored after a failed restore, got %d", cn)
	}
}

func TestSegmentChangesCollectionNames(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	segmentC := NewSegmentChangesCollection(dbw, logger)
	segmentC.Update("s1", set.NewSet("k1"), set.NewSet(), 1)
	segmentC.Update("s2", set.NewSet("k2"), set.NewSet(), 2)
	segmentC.Update("s3", set.NewSet("k3"), set.NewSet(), 3)

	names := segmentC.Names()
	expected := map[string]bool{"s1": true, "s2": true, "s3": true}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected name %q", name)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		t.Errorf("missing names: %v", expected)
	}

	for i := range names {
		names[i] = "mutated"
	}
	after := segmentC.Names()
	for _, name := range after {
		if name == "mutated" {
			t.Errorf("mutating returned slice affected the collection")
		}
	}
	if len(after) != 3 {
		t.Errorf("expected 3 names after mutation, got %d", len(after))
	}
}

type fetchByCollection struct {
	CollectionWrapper
	data   []byte
	logger logging.LoggerInterface
}

func (f *fetchByCollection) FetchBy(key []byte) ([]byte, error) { return f.data, nil }

func (f *fetchByCollection) Logger() logging.LoggerInterface { return f.logger }

func TestSegmentChangesCollectionFetchDecodeErrorIsReturned(t *testing.T) {
	logger := logging.NewLogger(nil)
	c := &SegmentChangesCollectionImpl{
		collection:   &fetchByCollection{data: []byte("not gob"), logger: logger},
		segmentsTill: make(map[string]int64),
		logger:       logger,
	}
	item, err := c.Fetch("seg")
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if item != nil {
		t.Errorf("expected no item on decode error, got %+v", item)
	}
}

func TestRestoreEmptySegmentNameReturnsError(t *testing.T) {
	dbw, err := NewBoltWrapper(t.TempDir()+"/state.db", nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}
	defer dbw.Discard()

	segmentC := NewSegmentChangesCollection(dbw, logging.NewLogger(nil))
	err = segmentC.Restore(SegmentChangesItem{Name: "", Keys: map[string]SegmentKey{"k1": {Name: "k1", ChangeNumber: 3}}})
	if err == nil {
		t.Fatal("restore with an empty segment name must return an error")
	}
	for _, name := range segmentC.Names() {
		if name == "" {
			t.Error("empty segment name must not be recorded as a restored segment")
		}
	}
}

func TestFailedSaveAsLeavesSegmentsTillUnchanged(t *testing.T) {
	dbw, err := NewBoltWrapper(t.TempDir()+"/state.db", nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}

	segmentC := NewSegmentChangesCollection(dbw, logging.NewLogger(nil))
	if err := segmentC.Update("s1", set.NewSet("k1"), set.NewSet(), 5); err != nil {
		t.Fatal("initial update should succeed: ", err)
	}

	if err := dbw.Discard(); err != nil {
		t.Fatal("error closing bolt: ", err)
	}

	if err := segmentC.Update("s1", set.NewSet("k2"), set.NewSet(), 7); err == nil {
		t.Fatal("update on a closed db must fail")
	}
	if cn := segmentC.ChangeNumber("s1"); cn != 5 {
		t.Errorf("segmentsTill must stay at 5 after a failed SaveAs, got %d", cn)
	}
}
