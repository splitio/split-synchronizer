package optimized

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
)

func TestMySegmentsV2(t *testing.T) {
	storage := NewMySegmentsCache()

	storage.Update("one", set.NewSet("test"), set.NewSet())
	storage.Update("two", set.NewSet("test"), set.NewSet())
	storage.Update("three", set.NewSet("test"), set.NewSet())
	storage.Update("three", set.NewSet("test"), set.NewSet())

	segments := storage.SegmentsForUser("test")
	if len(segments) != 3 {
		t.Error("It should have 3 segments")
	}

	if len(storage.SegmentsForUser("nonexistent")) != 0 {
		t.Error("It should be empty")
	}

	storage.Update("two", set.NewSet(), set.NewSet("test"))
	segments = storage.SegmentsForUser("test")
	if len(segments) != 2 {
		t.Error("It should have 2 segments")
	}

	storage.Update("three", set.NewSet(), set.NewSet("test"))
	segments = storage.SegmentsForUser("test")
	if len(segments) != 1 {
		t.Error("It should have 1 segments")
	}

	storage.Update("nonexistent", set.NewSet(), set.NewSet("test"))
	segments = storage.SegmentsForUser("test")
	if len(segments) != 1 {
		t.Error("It should have 1 segments")
	}

	storage.Update("one", set.NewSet(), set.NewSet("test"))
	segments = storage.SegmentsForUser("test")
	if len(segments) != 0 {
		t.Error("It should be empty")
	}

	storage.Update("one", set.NewSet(), set.NewSet("nonexistent"))
}

func TestMySegmentsAddWinsWhenInBothLists(t *testing.T) {
	storage := NewMySegmentsCache()
	storage.Update("one", set.NewSet("test"), set.NewSet())

	storage.Update("one", set.NewSet("test"), set.NewSet("test"))
	segments := storage.SegmentsForUser("test")
	if len(segments) != 1 || segments[0] != "one" {
		t.Error("key in both lists should keep the segment (add wins)")
	}
}

func TestMySegmentsAddOnly(t *testing.T) {
	storage := NewMySegmentsCache()
	storage.Update("one", set.NewSet("test"), set.NewSet())

	segments := storage.SegmentsForUser("test")
	if len(segments) != 1 || segments[0] != "one" {
		t.Error("key should have the segment after add")
	}
}

func TestMySegmentsRemoveOnly(t *testing.T) {
	storage := NewMySegmentsCache()
	storage.Update("one", set.NewSet("test"), set.NewSet())
	storage.Update("one", set.NewSet(), set.NewSet("test"))

	if len(storage.SegmentsForUser("test")) != 0 {
		t.Error("key should not have the segment after remove")
	}
}
