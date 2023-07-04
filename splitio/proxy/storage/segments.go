package storage

import (
	"errors"
	"fmt"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/storage"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
)

// ErrSegmentNotFound is returned when the segment whose changes we're querying isn't cached
var ErrSegmentNotFound = errors.New("segment not found")

// ProxySegmentStorage defines the set of methods that are required for the proxy server
// to respond to resquests from sdk clients
type ProxySegmentStorage interface {
	ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error)
	SegmentsFor(key string) ([]string, error)
	CountRemovedKeys(segmentName string) int
}

// ProxySegmentStorageImpl implements the ProxySegmentStorage interface
type ProxySegmentStorageImpl struct {
	logger         logging.LoggerInterface
	nameCountCache *observability.ActiveSegmentTracker
	db             *persistent.SegmentChangesCollection
	mysegments     optimized.MySegmentsCache
}

// NewProxySegmentStorage for proxy
func NewProxySegmentStorage(db persistent.DBWrapper, logger logging.LoggerInterface, restoreFromBackup bool) *ProxySegmentStorageImpl {
	cache := optimized.NewMySegmentsCache()
	disk := persistent.NewSegmentChangesCollection(db, logger)
	nameCountCache := observability.NewActiveSegmentTracker(100) // just a guess, we don't know the size yet
	if restoreFromBackup {
		populateCachesFromDisk(cache, nameCountCache, disk, logger)
	}
	return &ProxySegmentStorageImpl{
		db:             disk,
		mysegments:     cache,
		logger:         logger,
		nameCountCache: nameCountCache,
	}
}

// ChangesSince returns the `segmentChanges` like payload to from a certain CN to the last snapshot
// This method has one drawback. ALL the historically removed keys are always returned as part of the `removed` array,
// regardless whether the `since` parameter is old enough to require such removal or not.
// We should eventually see if it's worth taking an approach similar to the one in feature flags or not
func (s *ProxySegmentStorageImpl) ChangesSince(name string, since int64) (*dtos.SegmentChangesDTO, error) {
	item, err := s.db.Fetch(name)
	if err != nil {
		if errors.Is(err, persistent.ErrorBucketNotFound) || errors.Is(err, persistent.ErrorKeyNotFound) {
			return nil, ErrSegmentNotFound
		}
		return nil, fmt.Errorf("unexpected error when fetching segment '%s': %w", name, err)
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	till := since

	// Horrible loop borrowed from sdk-api
	for _, skey := range item.Keys {
		if skey.ChangeNumber <= since { // if the key was updated in a previous/current CN, we don't need to return it
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

	return &dtos.SegmentChangesDTO{Name: name, Since: since, Till: till, Added: added, Removed: removed}, nil
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
	return s.db.ChangeNumber(segment), nil
}

// SetChangeNumber method
func (s *ProxySegmentStorageImpl) SetChangeNumber(segment string, changeNumber int64) error {
	s.db.SetChangeNumber(segment, changeNumber)
	return nil
}

// Keys method
func (s *ProxySegmentStorageImpl) Keys(segmentName string) *set.ThreadUnsafeSet {
	toReturn := set.NewSet()
	changes, err := s.db.Fetch(segmentName)
	if err != nil {
		if errors.Is(err, persistent.ErrorBucketNotFound) || errors.Is(err, persistent.ErrorKeyNotFound) {
			s.logger.Error(fmt.Sprintf("segment %s not found. failed to fetch keys.", segmentName))
		} else {
			s.logger.Error(fmt.Sprintf("unexpected error when fetching segment keys for '%s': %s", segmentName, err.Error()))
		}
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
	errCache := s.mysegments.Update(name, toAdd, toRemove)
	errDB := s.db.Update(name, toAdd, toRemove, changeNumber)
	if errCache == nil && errDB == nil {
		s.nameCountCache.Update(name, toAdd.Size(), toRemove.Size())
		return nil
	}

	return fmt.Errorf("errors updating cache: %s || errors updating db: %s", errCache.Error(), errDB.Error())
}

// CountRemovedKeys method
func (s *ProxySegmentStorageImpl) CountRemovedKeys(segmentName string) int {
	segment, err := s.db.Fetch(segmentName)
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

// NamesAndCount returns a map of segment names to key count
func (s *ProxySegmentStorageImpl) NamesAndCount() map[string]int {
	return s.nameCountCache.NamesAndCount()
}

func populateCachesFromDisk(
	dst optimized.MySegmentsCache,
	names *observability.ActiveSegmentTracker,
	src *persistent.SegmentChangesCollection,
	logger logging.LoggerInterface,
) {
	all, err := src.FetchAll()
	if err != nil {
		logger.Error("error popoulating segment cache from disk. Cache will be empty!: ", err)
		return
	}

	for idx := range all {
		s := set.NewSet()
		count := 0
		for _, k := range all[idx].Keys {
			if !k.Removed {
				s.Add(k.Name)
				count++
			}
		}
		dst.Update(all[idx].Name, s, set.NewSet())
		names.Update(all[idx].Name, count, 0)
	}
}

var _ storage.SegmentStorage = (*ProxySegmentStorageImpl)(nil)
var _ observability.ObservableSegmentStorage = (*ProxySegmentStorageImpl)(nil)
