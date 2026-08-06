package observability

import (
	"errors"
	"fmt"
	"sync"

	"github.com/splitio/go-split-commons/v9/storage"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

// ErrIncompatibleSegmentStorage is returned when the supplied storage that not have the required methods
var ErrIncompatibleSegmentStorage = errors.New("supplied segment storage doesn't report errors")

// ObservableSegmentStorage builds on top of the SegmentStorage interface adding some observability methods
type ObservableSegmentStorage interface {
	storage.SegmentStorage
	NamesAndCount() map[string]int
}

// ObservableSegmentStorageImpl is an implementation of the ObservableSegmentStorage interface
type ObservableSegmentStorageImpl struct {
	extendedSegmentStorage
	counter *ActiveSegmentTracker
	logger  logging.LoggerInterface
}

// NewObservableSegmentStorage constructs and observable segment storage
func NewObservableSegmentStorage(
	logger logging.LoggerInterface,
	splitStorage storage.SplitStorage,
	toWrap storage.SegmentStorage,
) (*ObservableSegmentStorageImpl, error) {

	extended, ok := toWrap.(extendedSegmentStorage)
	if !ok {
		return nil, ErrIncompatibleSegmentStorage
	}

	segmentNames := splitStorage.SegmentNames()
	tracker := NewActiveSegmentTracker(segmentNames.Size() + 1)

	segmentNames.Each(func(i interface{}) bool {
		strName, ok := i.(string)
		if !ok {
			logger.Warning(fmt.Sprintf("non-string segment name fetched: '%+v'//'%T'. This is a bug, please report it.", i, i))
			return true
		}

		count, err := extended.Size(strName)
		if err != nil {
			logger.Warning(fmt.Sprintf("failed to get size for segment %s. This may introduce inconsistencies in observability endpoints", strName))
		}

		tracker.Update(strName, count, 0)
		return true
	})

	return &ObservableSegmentStorageImpl{
		extendedSegmentStorage: extended,
		counter:                tracker,
		logger:                 logger,
	}, nil
}

// Update updates the local segment cache and forwards the call to he underlying storage
func (s *ObservableSegmentStorageImpl) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, changeNumber int64) error {
	added, removed, err := s.UpdateWithSummary(name, toAdd, toRemove, changeNumber)
	if err != nil {
		s.logger.Error(fmt.Sprintf("something went wrong when updating segment '%s': %s", name, err.Error()))
	}
	s.counter.Update(name, added, removed)
	return nil
}

// NamesAndCount returns a map of segment names with the number of keys
func (s *ObservableSegmentStorageImpl) NamesAndCount() map[string]int {
	return s.counter.NamesAndCount()
}

// ActiveSegmentTracker accepts updates and keeps track of segment names & cardinality
type ActiveSegmentTracker struct {
	activeSegmentMap map[string]int
	mtx              sync.RWMutex
}

// NewActiveSegmentTracker constructs a new segment tracker
func NewActiveSegmentTracker(initialSize int) *ActiveSegmentTracker {
	return &ActiveSegmentTracker{
		activeSegmentMap: make(map[string]int, initialSize+1), // to avoid ever constructing a map of size 0
	}
}

// Update the segment name/key-count cache
func (t *ActiveSegmentTracker) Update(name string, added int, removed int) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	current, _ := t.activeSegmentMap[name]
	current = current + added - removed
	if current <= 0 {
		delete(t.activeSegmentMap, name)
		return
	}
	t.activeSegmentMap[name] = current
}

// NamesAndCount returns a map of segment names to key count
func (t *ActiveSegmentTracker) NamesAndCount() map[string]int {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	ret := make(map[string]int, len(t.activeSegmentMap))
	for name, count := range t.activeSegmentMap {
		ret[name] = count
	}
	return ret
}

type (
	extendedSegmentStorage interface {
		storage.SegmentStorage
		UpdateWithSummary(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, till int64) (added int, removed int, err error)
		Size(name string) (int, error)
	}
)

var _ ObservableSegmentStorage = (*ObservableSegmentStorageImpl)(nil)
var _ storage.SegmentStorage = (*ObservableSegmentStorageImpl)(nil)
