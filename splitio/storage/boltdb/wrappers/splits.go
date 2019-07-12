package wrappers

import (
	"errors"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb/collections"
)

var errSplitStorageNotImplementedMethod = errors.New("Method has not been implemented yet")

//SplitChangesWrapper implements SplitStorage interface
type SplitChangesWrapper struct {
	splitCollection collections.SplitChangesCollection
}

// NewSplitChangesWrapper returns an instance of SplitChangesWrapper
func NewSplitChangesWrapper() *SplitChangesWrapper {
	return &SplitChangesWrapper{splitCollection: collections.NewSplitChangesCollection(boltdb.DBB)}
}

// Save not implemented due this wrapper is only for dashboard
func (s *SplitChangesWrapper) Save(split interface{}) error {
	return errSplitStorageNotImplementedMethod

}

// Remove not implemented due this wrapper is only for dashboard
func (s *SplitChangesWrapper) Remove(split interface{}) error {
	return errSplitStorageNotImplementedMethod
}

// RegisterSegment not implemented due this wrapper is only for dashboard
func (s *SplitChangesWrapper) RegisterSegment(name string) error {
	return errSplitStorageNotImplementedMethod
}

// SetChangeNumber not implemented due this wrapper is only for dashboard
func (s *SplitChangesWrapper) SetChangeNumber(changeNumber int64) error {
	return errSplitStorageNotImplementedMethod
}

// ChangeNumber not implemented due this wrapper is only for dashboard
func (s *SplitChangesWrapper) ChangeNumber() (int64, error) {
	return 0, errSplitStorageNotImplementedMethod
}

// SplitsNames fetchs splits names from redis
func (s *SplitChangesWrapper) SplitsNames() ([]string, error) {
	toReturn := make([]string, 0)
	splits, err := s.splitCollection.FetchAll()
	if err != nil {
		log.Error.Println("Error fetching splits from boltdb")
		return nil, err
	}

	for _, split := range splits {
		toReturn = append(toReturn, split.Name)
	}

	return toReturn, nil

}

// RawSplits return an slice with Split json representation
func (s *SplitChangesWrapper) RawSplits() ([]string, error) {
	toReturn := make([]string, 0)
	splits, err := s.splitCollection.FetchAll()
	if err != nil {
		log.Error.Println("Error fetching splits from boltdb")
		return nil, err
	}

	for _, split := range splits {
		toReturn = append(toReturn, split.JSON)
	}

	return toReturn, nil
}
