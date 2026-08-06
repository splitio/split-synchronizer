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
