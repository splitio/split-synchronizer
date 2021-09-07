package optimized

import "testing"

func TestMySegmentsV2(t *testing.T) {
	storage := NewMySegmentsCache()

	storage.AddSegmentToUser("test", "one")
	storage.AddSegmentToUser("test", "two")
	storage.AddSegmentToUser("test", "three")
	storage.AddSegmentToUser("test", "three")

	segments := storage.SegmentsForUser("test")
	if len(segments) != 3 {
		t.Error("It should have 3 segments")
	}

	if len(storage.SegmentsForUser("nonexistent")) != 0 {
		t.Error("It should be empty")
	}

	storage.RemoveSegmentForUser("test", "two")
	segments = storage.SegmentsForUser("test")
	if len(segments) != 2 {
		t.Error("It should have 2 segments")
	}

	storage.RemoveSegmentForUser("test", "three")
	segments = storage.SegmentsForUser("test")
	if len(segments) != 1 {
		t.Error("It should have 1 segments")
	}

	storage.RemoveSegmentForUser("test", "nonexistent")
	segments = storage.SegmentsForUser("test")
	if len(segments) != 1 {
		t.Error("It should have 1 segments")
	}

	storage.RemoveSegmentForUser("test", "one")
	segments = storage.SegmentsForUser("test")
	if len(segments) != 0 {
		t.Error("It should be empty")
	}

	storage.RemoveSegmentForUser("nonexistent", "one")
}
