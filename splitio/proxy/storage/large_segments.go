package storage

import (
	"sort"
	"sync"

	"github.com/splitio/go-toolkit/v5/logging"
)

// LargeSegmentsStorage defines the interface for a per-user large segments storage
type LargeSegmentsStorage interface {
	Count() int
	LargeSegmentsForUser(userKey string) []string
	Update(lsName string, userKeys []string)
}

// LargeSegmentsStorageImpl implements the LargeSegmentsStorage interface
type LargeSegmentsStorageImpl struct {
	largeSegments map[string][]string
	mutex         *sync.RWMutex
	logger        logging.LoggerInterface
}

// NewLargeSegmentsStorage constructs a new LargeSegments cache
func NewLargeSegmentsStorage(logger logging.LoggerInterface) *LargeSegmentsStorageImpl {
	return &LargeSegmentsStorageImpl{
		largeSegments: make(map[string][]string),
		mutex:         &sync.RWMutex{},
		logger:        logger,
	}
}

// Count retuns the amount of Large Segments
func (s *LargeSegmentsStorageImpl) Count() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.largeSegments)
}

// SegmentsForUser returns the list of segments a certain user belongs to
func (s *LargeSegmentsStorageImpl) LargeSegmentsForUser(userKey string) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	toReturn := make([]string, 0, len(s.largeSegments))
	for lsName, data := range s.largeSegments {
		i := sort.Search(len(data), func(i int) bool {
			return data[i] >= userKey
		})

		if i < len(data) && data[i] == userKey {
			toReturn = append(toReturn, lsName)
		}
	}

	return toReturn
}

// Update adds and remove keys to segments
func (s *LargeSegmentsStorageImpl) Update(lsName string, userKeys []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.largeSegments[lsName] = userKeys
}

var _ LargeSegmentsStorage = (*LargeSegmentsStorageImpl)(nil)
