package storage

import (
	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/splitio/storage/redis"
)

// SegmentStorageMainFactory factory for SegmentStorage
type SegmentStorageMainFactory struct {
}

// NewInstance returns an instance of implemented SegmentStorage interface
func (f SegmentStorageMainFactory) NewInstance() SegmentStorage {
	return redis.NewSegmentStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}
