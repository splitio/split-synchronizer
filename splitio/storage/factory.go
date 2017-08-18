package storage

import (
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

// SegmentStorageMainFactory factory for SegmentStorage
type SegmentStorageMainFactory struct {
}

// NewInstance returns an instance of implemented SegmentStorage interface
func (f SegmentStorageMainFactory) NewInstance() SegmentStorage {
	return redis.NewSegmentStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}
