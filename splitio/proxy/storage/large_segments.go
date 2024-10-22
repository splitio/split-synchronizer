package storage

import (
	"sort"
	"sync"

	"github.com/splitio/go-toolkit/v5/logging"
)

type LargeSegmentsStorage interface {
	Count() int
	SegmentsForUser(key string) []string
	Update(lsName string, userKeys []string)
}

// MySegmentsCacheImpl implements the MySegmentsCache interface
type LargeSegmentsStorageImpl struct {
	largeSegments map[string][]string
	mutex         *sync.RWMutex
	logger        logging.LoggerInterface
}

// NewMySegmentsCache constructs a new MySegments cache
func NewLargeSegmentsStorage(logger logging.LoggerInterface) *LargeSegmentsStorageImpl {
	return &LargeSegmentsStorageImpl{
		largeSegments: make(map[string][]string),
		mutex:         &sync.RWMutex{},
		logger:        logger,
	}
}

func (s *LargeSegmentsStorageImpl) Count() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.largeSegments)
}

func (s *LargeSegmentsStorageImpl) SegmentsForUser(key string) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	toReturn := make([]string, 0)
	lsNames := s.names()

	for _, name := range lsNames {
		if s.exists(name, key) {
			toReturn = append(toReturn, name)
		}
	}

	return toReturn
}

func (s *LargeSegmentsStorageImpl) Update(lsName string, userKeys []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.largeSegments[lsName] = userKeys
}

func (s *LargeSegmentsStorageImpl) names() []string {
	toReturn := make([]string, 0, len(s.largeSegments))
	for key := range s.largeSegments {
		toReturn = append(toReturn, key)
	}

	return toReturn
}

func (s *LargeSegmentsStorageImpl) exists(lsName string, userKey string) bool {
	data := s.largeSegments[lsName]
	length := len(data)
	if length == 0 {
		return false
	}

	i := sort.Search(length, func(i int) bool {
		return data[i] >= userKey
	})

	return i < len(data) && data[i] == userKey
}
