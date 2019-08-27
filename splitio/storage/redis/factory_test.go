package redis

import (
	"testing"
)

func TestSegmentStorageFactory(t *testing.T) {
	segmentStorageFactory := SegmentStorageMainFactory{}

	redisInstance := segmentStorageFactory.NewInstance()

	_, ok := redisInstance.(*SegmentStorageAdapter)
	if !ok {
		t.Error("Type Error")
	}
}
