package persistent

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	bolt "go.etcd.io/bbolt"
)

func newContentIDDB(t *testing.T) *BoltDBWrapper {
	t.Helper()
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatal("error creating bolt wrapper: ", err)
	}
	t.Cleanup(func() { dbw.wrapped.Close() })
	return dbw
}

func contentIDFlag(name string, cn int64, status string, treatment string) dtos.SplitDTO {
	return dtos.SplitDTO{Name: name, ChangeNumber: cn, Status: status, TrafficTypeName: "user", DefaultTreatment: treatment}
}

func contentIDRB(name string, cn int64, status string) dtos.RuleBasedSegmentDTO {
	return dtos.RuleBasedSegmentDTO{Name: name, ChangeNumber: cn, Status: status, TrafficTypeName: "user"}
}

func contentIDOf(t *testing.T, dbw *BoltDBWrapper, flagSpecVersion string, flagSets []string) string {
	t.Helper()
	_, id, err := dbw.GetRawSnapshotWithContentID(flagSpecVersion, flagSets)
	if err != nil {
		t.Fatal("GetRawSnapshotWithContentID should not return an error. Got: ", err)
	}
	return id
}

func newFlagsDB(t *testing.T, flags ...dtos.SplitDTO) *BoltDBWrapper {
	t.Helper()
	dbw := newContentIDDB(t)
	NewSplitChangesCollection(dbw, logging.NewLogger(nil)).Update(flags, nil, 1)
	return dbw
}

func TestContentIDSameLogicalFlagsDifferentHistory(t *testing.T) {
	logger := logging.NewLogger(nil)

	var all []dtos.SplitDTO
	for i := 0; i < 10; i++ {
		all = append(all, contentIDFlag(string(rune('a'+i))+"_flag", int64(i+1), "ACTIVE", "on"))
	}
	f3 := all[3]

	// db1: every flag in a single update
	db1 := newContentIDDB(t)
	NewSplitChangesCollection(db1, logger).Update(all, nil, 10)

	// db2: same flags, reversed and split across several updates, with f3 archived and re-added
	db2 := newContentIDDB(t)
	splits := NewSplitChangesCollection(db2, logger)
	var reversed []dtos.SplitDTO
	for i := len(all) - 1; i >= 5; i-- {
		reversed = append(reversed, all[i])
	}
	splits.Update(reversed, nil, 9)
	splits.Update([]dtos.SplitDTO{f3}, nil, 9)
	splits.Update([]dtos.SplitDTO{contentIDFlag(f3.Name, 20, "ARCHIVED", "on")}, nil, 20)
	splits.Update([]dtos.SplitDTO{f3}, nil, 21)
	splits.Update([]dtos.SplitDTO{all[0], all[1], all[2], all[4]}, nil, 21)

	id1 := contentIDOf(t, db1, "1.1", nil)
	id2 := contentIDOf(t, db2, "1.1", nil)
	if id1 != id2 {
		t.Error("same logical flags in different orders/groupings must yield the same content id")
	}
}

func TestContentIDChangedFlagChangesID(t *testing.T) {
	f1 := contentIDFlag("f1", 1, "ACTIVE", "on")
	f2 := contentIDFlag("f2", 2, "ACTIVE", "on")
	f3 := contentIDFlag("f3", 3, "ACTIVE", "on")

	base := contentIDOf(t, newFlagsDB(t, f1, f2, f3), "1.1", nil)

	changedTreatment := contentIDOf(t, newFlagsDB(t, f1, contentIDFlag("f2", 2, "ACTIVE", "off"), f3), "1.1", nil)
	if base == changedTreatment {
		t.Error("changing a treatment should change the content id")
	}

	changedCN := contentIDOf(t, newFlagsDB(t, f1, contentIDFlag("f2", 5, "ACTIVE", "on"), f3), "1.1", nil)
	if base == changedCN {
		t.Error("changing a changeNumber should change the content id")
	}
}

func TestContentIDFlagTombstones(t *testing.T) {
	logger := logging.NewLogger(nil)
	g1 := contentIDFlag("g1", 1, "ACTIVE", "on")
	x := contentIDFlag("x", 1, "ACTIVE", "on")
	xArchived := contentIDFlag("x", 2, "ARCHIVED", "on")

	// db1: x created then archived
	db1 := newContentIDDB(t)
	s1 := NewSplitChangesCollection(db1, logger)
	s1.Update([]dtos.SplitDTO{g1, x}, nil, 1)
	s1.Update([]dtos.SplitDTO{xArchived}, nil, 2)

	// db2: x never seen
	db2 := newFlagsDB(t, g1)

	// db3: x active
	db3 := newFlagsDB(t, g1, x)

	id1 := contentIDOf(t, db1, "1.1", nil)
	id2 := contentIDOf(t, db2, "1.1", nil)
	id3 := contentIDOf(t, db3, "1.1", nil)

	if id1 != id2 {
		t.Error("archived tombstone must contribute nothing: created-then-archived should equal never-seen")
	}
	if id1 == id3 {
		t.Error("archived flag must differ from the same flag active")
	}
}

func TestContentIDRuleBasedSegmentTombstones(t *testing.T) {
	logger := logging.NewLogger(nil)
	rb1 := contentIDRB("rb1", 1, "ACTIVE")
	rbX := contentIDRB("rbx", 1, "ACTIVE")
	rbXArchived := contentIDRB("rbx", 2, "ARCHIVED")

	// db1: rbx created then archived
	db1 := newContentIDDB(t)
	rb1Coll := NewRBChangesCollection(db1, logger)
	rb1Coll.Update([]dtos.RuleBasedSegmentDTO{rb1, rbX}, nil, 1)
	rb1Coll.Update([]dtos.RuleBasedSegmentDTO{rbXArchived}, nil, 2)

	// db2: rbx never seen
	db2 := newContentIDDB(t)
	NewRBChangesCollection(db2, logger).Update([]dtos.RuleBasedSegmentDTO{rb1}, nil, 1)

	// db3: rbx active
	db3 := newContentIDDB(t)
	NewRBChangesCollection(db3, logger).Update([]dtos.RuleBasedSegmentDTO{rb1, rbX}, nil, 1)

	// db4: rbx active then archived
	db4 := newContentIDDB(t)
	rb4Coll := NewRBChangesCollection(db4, logger)
	rb4Coll.Update([]dtos.RuleBasedSegmentDTO{rb1, rbX}, nil, 1)
	rb4Coll.Update([]dtos.RuleBasedSegmentDTO{rbXArchived}, nil, 2)

	id1 := contentIDOf(t, db1, "1.1", nil)
	id2 := contentIDOf(t, db2, "1.1", nil)
	id3 := contentIDOf(t, db3, "1.1", nil)
	id4 := contentIDOf(t, db4, "1.1", nil)

	if id1 != id2 {
		t.Error("archived rule-based segment tombstone must contribute nothing")
	}
	if id3 == id4 {
		t.Error("active rule-based segment must differ from the same one archived")
	}
}

func TestContentIDRuleBasedSegmentContentChanges(t *testing.T) {
	logger := logging.NewLogger(nil)
	rb := contentIDRB("rb1", 1, "ACTIVE")

	rbDB := func(dto dtos.RuleBasedSegmentDTO) *BoltDBWrapper {
		dbw := newContentIDDB(t)
		NewRBChangesCollection(dbw, logger).Update([]dtos.RuleBasedSegmentDTO{dto}, nil, dto.ChangeNumber)
		return dbw
	}

	base := contentIDOf(t, rbDB(rb), "1.1", nil)

	withExcluded := rb
	withExcluded.Excluded = dtos.ExcludedDTO{Keys: []string{"k1"}}
	if base == contentIDOf(t, rbDB(withExcluded), "1.1", nil) {
		t.Error("changing a rule-based segment's content should change the content id")
	}

	if base == contentIDOf(t, rbDB(contentIDRB("rb1", 5, "ACTIVE")), "1.1", nil) {
		t.Error("changing a rule-based segment's changeNumber should change the content id")
	}
}

func TestContentIDSegmentKeys(t *testing.T) {
	logger := logging.NewLogger(nil)

	// dbRemoved: k1 and k2 added, then k2 removed
	dbRemoved := newContentIDDB(t)
	segRemoved := NewSegmentChangesCollection(dbRemoved, logger)
	segRemoved.Update("s1", set.NewSet("k1", "k2"), set.NewSet(), 1)
	segRemoved.Update("s1", set.NewSet(), set.NewSet("k2"), 2)

	// dbOnlyK1: only k1 added
	dbOnlyK1 := newContentIDDB(t)
	NewSegmentChangesCollection(dbOnlyK1, logger).Update("s1", set.NewSet("k1"), set.NewSet(), 1)

	if contentIDOf(t, dbRemoved, "1.1", nil) != contentIDOf(t, dbOnlyK1, "1.1", nil) {
		t.Error("removed keys must not contribute to the content id")
	}

	// dbAllRemoved: segment exists but every key was removed
	dbAllRemoved := newContentIDDB(t)
	segAllRemoved := NewSegmentChangesCollection(dbAllRemoved, logger)
	segAllRemoved.Update("s1", set.NewSet("k1"), set.NewSet(), 1)
	segAllRemoved.Update("s1", set.NewSet(), set.NewSet("k1"), 2)

	// dbNever: segment never seen
	dbNever := newContentIDDB(t)

	if contentIDOf(t, dbAllRemoved, "1.1", nil) == contentIDOf(t, dbNever, "1.1", nil) {
		t.Error("a segment with all keys removed is included by name and must differ from a never-seen segment")
	}

	// same keys, different insertion order and transaction grouping
	dbA := newContentIDDB(t)
	segA := NewSegmentChangesCollection(dbA, logger)
	segA.Update("s1", set.NewSet("k2"), set.NewSet(), 1)
	segA.Update("s1", set.NewSet("k1", "k3"), set.NewSet(), 2)

	dbB := newContentIDDB(t)
	NewSegmentChangesCollection(dbB, logger).Update("s1", set.NewSet("k3", "k1", "k2"), set.NewSet(), 1)

	if contentIDOf(t, dbA, "1.1", nil) != contentIDOf(t, dbB, "1.1", nil) {
		t.Error("same segment keys in different orders/transactions should yield the same content id")
	}

	// dbReAdded: k2 added, removed, then re-added. Its final state matches dbAddedOnly
	dbReAdded := newContentIDDB(t)
	segReAdded := NewSegmentChangesCollection(dbReAdded, logger)
	segReAdded.Update("s1", set.NewSet("k1", "k2"), set.NewSet(), 1)
	segReAdded.Update("s1", set.NewSet(), set.NewSet("k2"), 2)
	segReAdded.Update("s1", set.NewSet("k2"), set.NewSet(), 3)

	dbAddedOnly := newContentIDDB(t)
	NewSegmentChangesCollection(dbAddedOnly, logger).Update("s1", set.NewSet("k1", "k2"), set.NewSet(), 1)

	if contentIDOf(t, dbReAdded, "1.1", nil) != contentIDOf(t, dbAddedOnly, "1.1", nil) {
		t.Error("a key removed then re-added must yield the same content id as a key only added")
	}
}

func TestContentIDSpecVersionAndFlagSets(t *testing.T) {
	dbw := newFlagsDB(t, contentIDFlag("f1", 1, "ACTIVE", "on"))

	base := contentIDOf(t, dbw, "1.1", []string{"a", "b"})

	if base == contentIDOf(t, dbw, "1.2", []string{"a", "b"}) {
		t.Error("different flagSpecVersion must yield a different content id")
	}

	if base != contentIDOf(t, dbw, "1.1", []string{"b", "a"}) {
		t.Error("flagSets in different order must yield the same content id")
	}

	if base == contentIDOf(t, dbw, "1.1", []string{"a", "c"}) {
		t.Error("different flagSets must yield a different content id")
	}

	if base == contentIDOf(t, dbw, "1.1", nil) {
		t.Error("empty flagSets must yield a different content id than non-empty flagSets")
	}

	sets := []string{"b", "a"}
	contentIDOf(t, dbw, "1.1", sets)
	if sets[0] != "b" || sets[1] != "a" {
		t.Error("computing the content id must not mutate the caller's flagSets slice")
	}
}

func TestContentIDFlagSetsNormalized(t *testing.T) {
	dbw := newFlagsDB(t, contentIDFlag("f1", 1, "ACTIVE", "on"))

	if contentIDOf(t, dbw, "1.1", []string{"a", "a"}) != contentIDOf(t, dbw, "1.1", []string{"a"}) {
		t.Error("duplicate flag sets must yield the same content id as the deduplicated set")
	}
	if contentIDOf(t, dbw, "1.1", []string{"b", "a"}) != contentIDOf(t, dbw, "1.1", []string{"a", "b"}) {
		t.Error("flag sets in different orders must yield the same content id")
	}
	if contentIDOf(t, dbw, "1.1", []string{"a", "b"}) == contentIDOf(t, dbw, "1.1", []string{"a"}) {
		t.Error("different flag sets must yield a different content id")
	}
}

func TestGetRawSnapshotWithContentID(t *testing.T) {
	dbw := newFlagsDB(t,
		contentIDFlag("f1", 1, "ACTIVE", "on"),
		contentIDFlag("f2", 2, "ACTIVE", "off"),
	)
	NewSegmentChangesCollection(dbw, logging.NewLogger(nil)).Update("s1", set.NewSet("k1"), set.NewSet(), 1)

	raw, id, err := dbw.GetRawSnapshotWithContentID("1.1", []string{"a"})
	if err != nil {
		t.Fatal("GetRawSnapshotWithContentID should not return an error. Got: ", err)
	}

	plain, err := dbw.GetRawSnapshot()
	if err != nil {
		t.Fatal("GetRawSnapshot should not return an error. Got: ", err)
	}

	if len(raw) == 0 || len(raw) != len(plain) {
		t.Errorf("raw snapshot length mismatch: with id=%d plain=%d", len(raw), len(plain))
	}

	if len(id) != 64 {
		t.Errorf("content id should be 64 hex chars, got %d", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Error("content id should be hex: ", err)
	}
	if id != strings.ToLower(id) {
		t.Error("content id should be lowercase hex")
	}

	_, id2, err := dbw.GetRawSnapshotWithContentID("1.1", []string{"a"})
	if err != nil {
		t.Fatal("GetRawSnapshotWithContentID should not return an error. Got: ", err)
	}
	if id != id2 {
		t.Error("content id should be stable across calls on an unchanged db")
	}
}

func TestContentIDEmptyDatabase(t *testing.T) {
	id1 := contentIDOf(t, newContentIDDB(t), "1.1", nil)
	id2 := contentIDOf(t, newContentIDDB(t), "1.1", nil)

	if len(id1) != 64 {
		t.Error("an empty db should still produce a content id")
	}
	if id1 != id2 {
		t.Error("two empty dbs should produce the same content id")
	}
}

func TestContentIDDecodeErrorReturnsError(t *testing.T) {
	dbw := newContentIDDB(t)
	err := dbw.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(splitChangesCollectionName))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("broken"), []byte("garbage"))
	})
	if err != nil {
		t.Fatal("error writing corrupt item: ", err)
	}

	if _, _, err := dbw.GetRawSnapshotWithContentID("1.1", nil); err == nil {
		t.Error("a corrupt stored item must return an error")
	}
}
