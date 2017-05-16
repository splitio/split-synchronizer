package redis

import (
	"github.com/splitio/go-agent/log"
	redis "gopkg.in/redis.v5"
)

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

// AddToSegment adds a list of keys (strings)
func (r SegmentStorageAdapter) AddToSegment(segmentName string, keys []string) error {
	log.Debug.Println("Adding to segment", segmentName)
	if len(keys) == 0 {
		return nil
	}
	_keys := make([]interface{}, len(keys))
	for i, v := range keys {
		_keys[i] = v
	}
	log.Verbose.Println(_keys...)
	return r.client.SAdd(r.segmentNamespace(segmentName), _keys...).Err()
}

// RemoveFromSegment removes a list of keys (strings)
func (r SegmentStorageAdapter) RemoveFromSegment(segmentName string, keys []string) error {
	log.Debug.Println("Removing from segment", segmentName)
	if len(keys) == 0 {
		return nil
	}
	_keys := make([]interface{}, len(keys))
	for i, v := range keys {
		_keys[i] = v
	}
	log.Verbose.Println(_keys...)
	return r.client.SRem(r.segmentNamespace(segmentName), _keys...).Err()
}

// SetChangeNumber sets the till value belong to segmentName
func (r SegmentStorageAdapter) SetChangeNumber(segmentName string, changeNumber int64) error {
	return r.client.Set(r.segmentTillNamespace(segmentName), changeNumber, 0).Err()
}

// ChangeNumber gets the till value belong to segmentName
func (r SegmentStorageAdapter) ChangeNumber(segmentName string) (int64, error) {
	return r.client.Get(r.segmentTillNamespace(segmentName)).Int64()
}
