package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

func TestSegmentStorageFactory(t *testing.T) {
	segmentStorageFactory := SegmentStorageMainFactory{}

	redisInstance := segmentStorageFactory.NewInstance()

	_, ok := redisInstance.(*redis.SegmentStorageAdapter)
	if !ok {
		t.Error("Type Error")
	}
}
