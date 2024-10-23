package storage

import (
	"sort"
	"sync"

	"github.com/splitio/go-toolkit/v5/logging"
)

// LargeSegmentsStorage defines the interface for a per-user large segments storage
type LargeSegmentsStorage interface {
	Count() int
	SegmentsForUser(key string) []string
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

// Update adds and remove keys to segments
func (s *LargeSegmentsStorageImpl) Update(lsName string, userKeys []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.largeSegments[lsName] = userKeys
}

// names returns the list with Large Segment Names
func (s *LargeSegmentsStorageImpl) names() []string {
	toReturn := make([]string, 0, len(s.largeSegments))
	for key := range s.largeSegments {
		toReturn = append(toReturn, key)
	}

	return toReturn
}

// exists returns true if a userKey is part of a large segment, else returns false
func (s *LargeSegmentsStorageImpl) exists(lsName string, userKey string) bool {
	data, ok := s.largeSegments[lsName]
	if !ok {
		return false
	}

	i := sort.Search(len(data), func(i int) bool {
		return data[i] >= userKey
	})

	return i < len(data) && data[i] == userKey
}
