package storage

import (
	"errors"
	"sync"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
)

// ErrIncompatibleSplitStorage is returned when the supplied storage that not have the required methods
var ErrIncompatibleSplitStorage = errors.New("supplied split storage doesn't report errors")

// ObservableSplitStorage is an interface extender that adds the method `Count` to the split storage
type ObservableSplitStorage interface {
	storage.SplitStorage
	Count() int
	Names() []string
}

// ObservableSplitStorageImpl is an implementaion of the ObservableSplitStorage inteface that wraps an existing storage
// caches and caches splitnames in-memory (in case the underlying one is non-local, ie: redis)
type ObservableSplitStorageImpl struct {
	storage.SplitStorage
	active *activeSplitTracker
}

// NewObservableSplitStorage constructs a NewObservableSplitStorage
func NewObservableSplitStorage(toWrap storage.SplitStorage, logger logging.LoggerInterface) (*ObservableSplitStorageImpl, error) {

	names := toWrap.SplitNames()
	active := newActiveSplitTracker(len(names))
	active.update(names, nil)

	if _, ok := toWrap.(supportsUpdateWithErrors); !ok {
		return nil, ErrIncompatibleSplitStorage
	}

	return &ObservableSplitStorageImpl{
		SplitStorage: toWrap,
		active:       active,
	}, nil
}

// Update is an override that wraps the original Update method and calls update on the local cache as well
func (s *ObservableSplitStorageImpl) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) {
	if rs, ok := s.SplitStorage.(supportsUpdateWithErrors); ok {
		if err := rs.UpdateWithErrors(toAdd, toRemove, changeNumber); err != nil {
			switch parsedErr := err.(type) {
			case nil:
				// no error
			case *redis.UpdateError:
				toAdd = filterFailed(toAdd, parsedErr.FailedToAdd)
				toRemove = filterFailed(toRemove, parsedErr.FailedToRemove)
			default:
				// Other types of error are considered critical, meaning nothing got updated,
				// hence our cache should not be updated as well
				return
			}
		}
	} else {
		// TODO(mredolatti): is this worth logging? are we just going to annoy people?
		s.SplitStorage.Update(toAdd, toRemove, changeNumber)
	}
	s.active.update(splitNames(toAdd), splitNames(toRemove))
}

// Count returns the number of active splits
func (s *ObservableSplitStorageImpl) Count() int {
	return s.active.count()
}

// Names returns a list of cached splits
func (s *ObservableSplitStorageImpl) Names() []string {
	return s.active.names()
}

type activeSplitTracker struct {
	activeSplitMap map[string]struct{}
	mtx            sync.RWMutex
}

func newActiveSplitTracker(initialSize int) *activeSplitTracker {
	return &activeSplitTracker{
		activeSplitMap: make(map[string]struct{}, initialSize+1), // to avoid ever constructing a map of size 0
	}
}

func (t *activeSplitTracker) update(toAdd []string, toRemove []string) {
	t.mtx.Lock()
	for _, name := range toAdd {
		t.activeSplitMap[name] = struct{}{}
	}

	for _, name := range toRemove {
		delete(t.activeSplitMap, name)
	}
	t.mtx.Unlock()
}

func (t *activeSplitTracker) count() int {
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	return len(t.activeSplitMap)
}

func (t *activeSplitTracker) names() []string {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	ret := make([]string, 0, len(t.activeSplitMap))
	for name := range t.activeSplitMap {
		ret = append(ret, name)
	}
	return ret
}

func splitNames(splits []dtos.SplitDTO) []string {
	names := make([]string, 0, len(splits))
	for idx := range splits {
		names = append(names, splits[idx].Name)
	}
	return names
}

func filterFailed(in []dtos.SplitDTO, failed map[string]error) []dtos.SplitDTO {
	if len(failed) == 0 {
		return in
	}

	idx := 0
	newSliceEnd := len(in)
	for idx < newSliceEnd {
		if _, ok := failed[in[idx].Name]; !ok {
			// If this item isn't a failed one, keep going
			idx++
			continue
		}

		// Otherwise, replace it with the last one and shrink the size of the slice
		// idx is not updated since the previously-last element might also be a failed one, so needs to be checked
		// in the next iteration...
		newSliceEnd--
		in[idx] = in[newSliceEnd]
	}

	return in[:newSliceEnd]
}

type supportsUpdateWithErrors interface {
	UpdateWithErrors(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) error
}

var _ ObservableSplitStorage = (*ObservableSplitStorageImpl)(nil)
var _ storage.SplitStorage = (*ObservableSplitStorageImpl)(nil)
