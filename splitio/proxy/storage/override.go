package storage

import (
	"errors"
	"sync"

	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v6/dtos"
)

var (
	// ErrFeatureFlagNotFound is returned when a feature flag with the specified name does not exist
	ErrFeatureFlagNotFound = errors.New("feature flag not found")
)

// OverrideStorage defines the interface for managing overrides
type OverrideStorage interface {
	FF(name string) *dtos.SplitDTO
	OverrideFF(name string, killed *bool, defaultTreatment *string) (*dtos.SplitDTO, error)
	RemoveOverrideFF(name string)
}

// OverrideStorageImpl is an in-memory implementation of the OverrideStorage interface
type OverrideStorageImpl struct {
	ffStorage ProxySplitStorage

	ffOverrides      map[string]*dtos.SplitDTO
	ffOverridesMutex *sync.RWMutex
	cache            gincache.CacheFlusher
}

// NewOverrideStorage creates a new instance of OverrideStorageImpl
func NewOverrideStorage(
	ffStorage ProxySplitStorage,
	cache gincache.CacheFlusher,
) *OverrideStorageImpl {
	return &OverrideStorageImpl{
		ffStorage: ffStorage,
		cache:     cache,

		ffOverrides:      make(map[string]*dtos.SplitDTO),
		ffOverridesMutex: &sync.RWMutex{},
	}
	// ffname
	// 	  	till(original+1)
	//    	splitDTO

	// userKey
	// 		operation (added/removed)
	// 		segmentName
	// 		till(original+1)
	//
}

func (s *OverrideStorageImpl) FF(name string) *dtos.SplitDTO {
	s.ffOverridesMutex.RLock()
	defer s.ffOverridesMutex.RUnlock()

	return s.ffOverrides[name]
}

// OverrideFF overrides a feature flag with the specified name, killed status, and default treatment
func (s *OverrideStorageImpl) OverrideFF(name string, killed *bool, defaultTreatment *string) (*dtos.SplitDTO, error) {
	s.ffOverridesMutex.Lock()
	defer s.ffOverridesMutex.Unlock()

	// Get the feature flag from the storage
	result := s.ffStorage.FetchMany([]string{name})
	if result == nil {
		return nil, ErrFeatureFlagNotFound
	}
	ff, exists := result[name]
	if !exists {
		return nil, ErrFeatureFlagNotFound
	}

	// Make updates
	if killed != nil {
		ff.Killed = *killed
	}
	if defaultTreatment != nil {
		ff.DefaultTreatment = *defaultTreatment
	}

	// Store the updated feature flag in the cache
	s.ffOverrides[name] = ff

	s.cache.EvictAll()

	return ff, nil
}

// RemoveOverrideFF removes a feature flag with the specified name
func (s *OverrideStorageImpl) RemoveOverrideFF(name string) {
	s.ffOverridesMutex.Lock()
	defer s.ffOverridesMutex.Unlock()

	delete(s.ffOverrides, name)

	s.cache.EvictAll()
}

var _ OverrideStorage = (*OverrideStorageImpl)(nil)
