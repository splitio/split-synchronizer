// Package storage implements different kind of storages for split information
package storage

import (
	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/splitio/storage/redis"
)

// SegmentStorageFactory factory for SegmentStorage
type SegmentStorageFactory struct {
}

// NewInstance returns an instance of implemented SegmentStorage interface
func (f SegmentStorageFactory) NewInstance() SegmentStorage {
	return redis.NewSegmentStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}
