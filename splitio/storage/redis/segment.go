// Package redis implements different kind of storages for split information
package redis

import redis "gopkg.in/redis.v5"

//RegisteredSegments() ([]interface{}, error)

// SegmentStorageAdapter implements SplitStorage interface
type SegmentStorageAdapter struct {
	*BaseStorageAdapter
}

// NewSegmentStorageAdapter returns an instance of Redis Segment adapter
func NewSegmentStorageAdapter(clientInstance *redis.Client, prefix string) *SegmentStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := SegmentStorageAdapter{adapter}
	return &client
}

// RegisteredSegmentNames returns a list of strings
func (r SegmentStorageAdapter) RegisteredSegmentNames() ([]string, error) {
	redisSegmentNames := r.client.SMembers(r.segmentsRegisteredNamespace())
	return redisSegmentNames.Val(), redisSegmentNames.Err()
}
