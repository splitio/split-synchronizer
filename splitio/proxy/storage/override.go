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

type SegmentOverride struct {
	Operation string `json:"operation"` // "Added" or "Removed"
}

type PerKey struct {
	Operation string `json:"operation"` // "Added" or "Removed"
	Key       string `json:"key"`       // User key
}

// OverrideStorage defines the interface for managing overrides
type OverrideStorage interface {
	GetOverrides() map[string]*dtos.SplitDTO
	FF(name string) *dtos.SplitDTO
	OverrideFF(name string, killed *bool, defaultTreatment *string) (*dtos.SplitDTO, error)
	RemoveOverrideFF(name string)

	GetOverridesForSegment() map[string][]PerKey
	Segment(key string) map[string]SegmentOverride
	OverrideSegment(key string, name string, operation string) SegmentOverride
	RemoveOverrideSegment(key string, name string)
}

// OverrideStorageImpl is an in-memory implementation of the OverrideStorage interface
type OverrideStorageImpl struct {
	ffStorage ProxySplitStorage

	ffOverrides      map[string]*dtos.SplitDTO
	ffOverridesMutex *sync.RWMutex
	cache            gincache.CacheFlusher

	segmentOverrides      map[string]map[string]SegmentOverride
	segmentOverridesMutex *sync.RWMutex
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

		segmentOverrides:      make(map[string]map[string]SegmentOverride),
		segmentOverridesMutex: &sync.RWMutex{},
	}
	// ffname
	// 	  	till(original+1)
	//    	splitDTO

	// userKey
	// 		operation (added/removed)
	// 		segmentName
	// 		till(original+1)
	//
	// 		Added, Segment1, Till+1
	// 		Removed, Segment2, Till+1
	// 		Added, Segment3, Till+1
}

// GetOverrides returns all feature flag overrides
func (s *OverrideStorageImpl) GetOverrides() map[string]*dtos.SplitDTO {
	s.ffOverridesMutex.RLock()
	defer s.ffOverridesMutex.RUnlock()
	return s.ffOverrides
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

// GetOverridesForSegment returns all segment overrides for a specific user key
func (s *OverrideStorageImpl) GetOverridesForSegment() map[string][]PerKey {
	s.segmentOverridesMutex.RLock()
	defer s.segmentOverridesMutex.RUnlock()

	toReturn := make(map[string][]PerKey)

	for key, segment := range s.segmentOverrides {
		if _, exists := toReturn[key]; !exists {
			toReturn[key] = []PerKey{}
		}
		// Iterate over each segment override and append it to the result
		for segmentName, override := range segment {
			toReturn[key] = append(toReturn[key], PerKey{
				Operation: override.Operation,
				Key:       segmentName,
			})
		}
	}
	return toReturn
}

// Segment returns the overrides for a specific user key
func (s *OverrideStorageImpl) Segment(key string) map[string]SegmentOverride {
	s.segmentOverridesMutex.RLock()
	defer s.segmentOverridesMutex.RUnlock()

	return s.segmentOverrides[key]
}

// OverrideSegment overrides a segment for a specific user key with the specified operation
func (s *OverrideStorageImpl) OverrideSegment(key string, name string, operation string) SegmentOverride {
	s.segmentOverridesMutex.Lock()
	defer s.segmentOverridesMutex.Unlock()

	if _, exists := s.segmentOverrides[key]; !exists {
		s.segmentOverrides[key] = make(map[string]SegmentOverride)
	}

	s.segmentOverrides[key][name] = SegmentOverride{Operation: operation}
	return SegmentOverride{Operation: operation}
}

// RemoveOverrideSegment removes a segment override for a specific user key
func (s *OverrideStorageImpl) RemoveOverrideSegment(key string, name string) {
	s.segmentOverridesMutex.Lock()
	defer s.segmentOverridesMutex.Unlock()

	if _, exists := s.segmentOverrides[key]; exists {
		delete(s.segmentOverrides[key], name)
	}

	if len(s.segmentOverrides[key]) == 0 {
		delete(s.segmentOverrides, key)
	}
}

var _ OverrideStorage = (*OverrideStorageImpl)(nil)
