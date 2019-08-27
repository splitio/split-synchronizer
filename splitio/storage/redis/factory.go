package redis

import (
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

// SegmentStorageMainFactory factory for SegmentStorage
type SegmentStorageMainFactory struct {
}

// NewInstance returns an instance of implemented SegmentStorage interface
func (f SegmentStorageMainFactory) NewInstance() storage.SegmentStorage {
	return NewSegmentStorageAdapter(Client, conf.Data.Redis.Prefix)
}
