package storage

import (
	"errors"
	"fmt"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/persistent"
)

// ProxySegmentStorage defines the set of methods that are required for the proxy server
// to respond to resquests from sdk clients
type ProxySegmentStorage interface {
	ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error)
	SegmentsFor(key string) ([]string, error)
	CountRemovedKeys(segmentName string) int
}

// ProxySegmentStorageImpl implements the ProxySegmentStorage interface
type ProxySegmentStorageImpl struct {
	logger     logging.LoggerInterface
	disk       *persistent.SegmentChangesCollection
	mysegments optimized.MySegmentsCache
}

// NewProxySegmentStorage for proxy
func NewProxySegmentStorage(db persistent.DBWrapper, logger logging.LoggerInterface) *ProxySegmentStorageImpl {
	return &ProxySegmentStorageImpl{
		disk:       persistent.NewSegmentChangesCollection(db, logger),
		mysegments: optimized.NewMySegmentsCache(),
		logger:     logger,
	}
}

// ChangesSince returns the `segmentChanges` like payload to from a certain CN to the last snapshot
// This method has one drawback. ALL the historically removed keys are always returned as part of the `removed` array,
// regardless whether the `since` parameter is old enough to require such removal or not.
// We should eventually see if it's worth taking an approach similar to the one in splits or not
func (s *ProxySegmentStorageImpl) ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error) {
	item, err := s.disk.Fetch(name)
	if err != nil {
		if errors.Is(err, persistent.ErrorBucketNotFound) {
			// Collection not yet created
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected error when fetching segment '%s': %w", name, err)
	}

	if item == nil {
		return nil, nil
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	till := since

	// Horrible loop borrowed from sdk-api
	for _, skey := range item.Keys {
		if skey.ChangeNumber < since {
			continue
		}

		// Add the key to the corresponding list
		if skey.Removed && since > 0 {
			removed = append(removed, skey.Name)
		} else {
			added = append(added, skey.Name)
		}

		// Update the till to be returned if necessary
		if since > 0 && skey.ChangeNumber > till {
			till = skey.ChangeNumber
		} else if !skey.Removed && skey.ChangeNumber > till {
			till = skey.ChangeNumber
		}
	}

	return &dtos.SegmentChangesDTO{Since: since, Till: till, Added: added, Removed: removed}, nil
}

// SegmentsFor returns the list of segments a key belongs to
func (s *ProxySegmentStorageImpl) SegmentsFor(key string) ([]string, error) {
	return s.mysegments.SegmentsForUser(key), nil
}

// SegmentKeysCount returns 0
func (s *ProxySegmentStorageImpl) SegmentKeysCount() int64 {
	return int64(s.mysegments.KeyCount())
}

// ChangeNumber storage
func (s *ProxySegmentStorageImpl) ChangeNumber(segment string) (int64, error) {
	return s.disk.ChangeNumber(segment), nil
}

// SetChangeNumber method
func (s *ProxySegmentStorageImpl) SetChangeNumber(segment string, changeNumber int64) error {
	s.disk.SetChangeNumber(segment, changeNumber)
	return nil
}

// Keys method
func (s *ProxySegmentStorageImpl) Keys(segmentName string) *set.ThreadUnsafeSet {
	toReturn := set.NewSet()
	changes, err := s.disk.Fetch(segmentName)
	if err != nil {
		if !errors.Is(err, persistent.ErrorBucketNotFound) {
			s.logger.Error(fmt.Sprintf("unexpected error when fetching segment keys for '%s': %s", segmentName, err.Error()))
		}
		return toReturn
	}

	if changes == nil {
		// Segment not cached
		return toReturn
	}

	for _, key := range changes.Keys {
		toReturn.Add(key)
	}
	return toReturn
}

// SegmentContainsKey method
func (s *ProxySegmentStorageImpl) SegmentContainsKey(segmentName string, key string) (bool, error) {
	return false, nil
}

// Update method
func (s *ProxySegmentStorageImpl) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, changeNumber int64) error {
	// TODO(mredolatti): lock!
	segmentItem, _ := s.disk.Fetch(name)

	if segmentItem == nil {
		segmentItem = &persistent.SegmentChangesItem{}
		segmentItem.Name = name
		segmentItem.Keys = make(map[string]persistent.SegmentKey)
	}

	for _, removedKey := range toRemove.List() {
		strKey, ok := removedKey.(string)
		if !ok {
			s.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %+v", name, strKey))
			continue
		}
		s.logger.Debug("Removing", strKey, "from", name)
		s.mysegments.RemoveSegmentForUser(strKey, name)
		if _, exists := segmentItem.Keys[strKey]; exists {
			itemAux := segmentItem.Keys[strKey]
			itemAux.Removed = true
			itemAux.ChangeNumber = changeNumber
			segmentItem.Keys[strKey] = itemAux
		} else {
			segmentItem.Keys[strKey] = persistent.SegmentKey{
				Name:         strKey,
				Removed:      true,
				ChangeNumber: changeNumber,
			}
		}

	}

	for _, addedKey := range toAdd.List() {
		strKey, ok := addedKey.(string)
		if !ok {
			s.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %+v", name, strKey))
			continue
		}
		s.logger.Debug("Adding", strKey, "in", name)
		s.mysegments.AddSegmentToUser(strKey, name)
		if _, exists := segmentItem.Keys[strKey]; exists {
			itemAux := segmentItem.Keys[strKey]
			itemAux.Removed = false
			itemAux.ChangeNumber = changeNumber
			segmentItem.Keys[strKey] = itemAux
		} else {
			segmentItem.Keys[strKey] = persistent.SegmentKey{
				Name:         strKey,
				Removed:      false,
				ChangeNumber: changeNumber,
			}
		}
	}

	err := s.disk.Add(segmentItem)
	if err != nil {
		return fmt.Errorf("error when updating persistant storage for segment %s: %w", name, err)
	}
	s.disk.SetChangeNumber(name, changeNumber)

	return nil
}

// CountRemovedKeys method
func (s *ProxySegmentStorageImpl) CountRemovedKeys(segmentName string) int {
	segment, err := s.disk.Fetch(segmentName)
	if err != nil {
		return 0
	}

	changeNumber := int64(0)
	removedKeys := 0
	for _, k := range segment.Keys {
		if k.ChangeNumber > changeNumber {
			changeNumber = k.ChangeNumber
		}

		if k.Removed {
			removedKeys++
		}
	}

	return removedKeys
}
