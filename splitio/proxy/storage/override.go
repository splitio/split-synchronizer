package storage

import (
	"errors"
	"sync"

	"github.com/splitio/go-split-commons/v6/dtos"
)

var (
	// ErrFeatureFlagNotFound is returned when a feature flag with the specified name does not exist
	ErrFeatureFlagNotFound = errors.New("feature flag not found")
)

// OverrideStorage defines the interface for managing overrides
type OverrideStorage interface {
	FeatureFlag(name string, killed *bool, defaultTreatment *string) (*dtos.SplitDTO, error)
	DeleteFeatureFlag(name string) (*dtos.SplitDTO, error)
}

// OverrideStorageImpl is an in-memory implementation of the OverrideStorage interface
type OverrideStorageImpl struct {
	ffStorage ProxySplitStorage

	ffDB    map[string]dtos.SplitDTO
	ffMutex sync.RWMutex
}

// NewOverrideStorage creates a new instance of OverrideStorageImpl
func NewOverrideStorage(
	ffStorage ProxySplitStorage,
) *OverrideStorageImpl {
	return &OverrideStorageImpl{
		ffStorage: ffStorage,

		ffDB:    make(map[string]dtos.SplitDTO),
		ffMutex: sync.RWMutex{},
	}
}

// FeatureFlag overrides a feature flag with the specified name, killed status, and default treatment
func (s *OverrideStorageImpl) FeatureFlag(name string, killed *bool, defaultTreatment *string) (*dtos.SplitDTO, error) {
	s.ffMutex.Lock()
	defer s.ffMutex.Unlock()

	result := s.ffStorage.FetchMany([]string{name})
	if result == nil {
		return nil, ErrFeatureFlagNotFound
	}
	ff, exists := result[name]
	if !exists {
		return nil, ErrFeatureFlagNotFound
	}

	if killed != nil {
		ff.Killed = *killed
	}
	if defaultTreatment != nil {
		ff.DefaultTreatment = *defaultTreatment
	}

	return ff, nil
}

// DeleteFeatureFlag removes a feature flag with the specified name
func (s *OverrideStorageImpl) DeleteFeatureFlag(name string) (*dtos.SplitDTO, error) {
	s.ffMutex.Lock()
	defer s.ffMutex.Unlock()

	result := s.ffStorage.FetchMany([]string{name})
	if result == nil {
		return nil, ErrFeatureFlagNotFound
	}
	ff, exists := result[name]
	if !exists {
		return nil, ErrFeatureFlagNotFound
	}

	delete(s.ffDB, name)
	return ff, nil
}

var _ OverrideStorage = (*OverrideStorageImpl)(nil)
