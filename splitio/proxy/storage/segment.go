package storage

import (
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-toolkit/v4/datastructures/set"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
)

// SegmentStorage struct
type SegmentStorage struct {
	segmentCollection collections.SegmentChangesCollection
}

// NewSegmentStorage for proxy
func NewSegmentStorage(segmentCollection collections.SegmentChangesCollection) storage.SegmentStorage {
	return SegmentStorage{
		segmentCollection: segmentCollection,
	}
}

// ChangeNumber storage
func (s SegmentStorage) ChangeNumber(segment string) (int64, error) {
	return s.segmentCollection.ChangeNumber(segment), nil
}

// SetChangeNumber method
func (s SegmentStorage) SetChangeNumber(segment string, changeNumber int64) error {
	s.segmentCollection.SetChangeNumber(segment, changeNumber)
	return nil
}

// Keys method
func (s SegmentStorage) Keys(segmentName string) *set.ThreadUnsafeSet {
	toReturn := set.NewSet()
	segmentChanges, _ := s.segmentCollection.Fetch(segmentName)
	for _, key := range segmentChanges.Keys {
		toReturn.Add(key)
	}
	return toReturn
}

// SegmentContainsKey method
func (s SegmentStorage) SegmentContainsKey(segmentName string, key string) (bool, error) {
	return false, nil
}

// Update method
func (s SegmentStorage) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, changeNumber int64) error {
	return nil
}

// CountRemovedKeys method
func (s SegmentStorage) CountRemovedKeys(segmentName string) int64 {
	segment, err := s.segmentCollection.Fetch(segmentName)
	if err != nil {
		return 0
	}

	changeNumber := int64(0)
	removedKeys := int64(0)
	addedKeys := int64(0)
	for _, k := range segment.Keys {
		if k.ChangeNumber > changeNumber {
			changeNumber = k.ChangeNumber
		}

		if k.Removed {
			removedKeys++
		} else {
			addedKeys++
		}
	}

	return removedKeys
}
