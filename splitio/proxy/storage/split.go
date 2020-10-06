package storage

import (
	"encoding/json"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-toolkit/v3/datastructures/set"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
)

// SplitStorage struct
type SplitStorage struct {
	splitCollection collections.SplitChangesCollection
}

// NewSplitStorage for proxy
func NewSplitStorage(splitCollection collections.SplitChangesCollection) storage.SplitStorage {
	return SplitStorage{
		splitCollection: splitCollection,
	}
}

// ChangeNumber storage
func (s SplitStorage) ChangeNumber() (int64, error) {
	return s.splitCollection.ChangeNumber(), nil
}

// SetChangeNumber method
func (s SplitStorage) SetChangeNumber(changeNumber int64) error {
	s.splitCollection.SetChangeNumber(changeNumber)
	return nil
}

// KillLocally kills
func (s SplitStorage) KillLocally(splitName string, defaultTreatment string, changeNumber int64) {}

// PutMany method
func (s SplitStorage) PutMany(splits []dtos.SplitDTO, changeNumber int64) {}

// Remove method
func (s SplitStorage) Remove(splitName string) {}

// All method
func (s SplitStorage) All() []dtos.SplitDTO {
	toReturn := make([]dtos.SplitDTO, 0)
	splitChanges, _ := s.splitCollection.FetchAll()
	for _, splitChange := range splitChanges {
		var split *dtos.SplitDTO
		err := json.Unmarshal([]byte(splitChange.JSON), &split)
		if err != nil {
			continue
		}
		toReturn = append(toReturn, *split)
	}
	return toReturn
}

// FetchMany method
func (s SplitStorage) FetchMany(splitNames []string) map[string]*dtos.SplitDTO {
	return map[string]*dtos.SplitDTO{}
}

// SegmentNames method
func (s SplitStorage) SegmentNames() *set.ThreadUnsafeSet { return s.splitCollection.SegmentNames() }

// Split method
func (s SplitStorage) Split(splitName string) *dtos.SplitDTO { return nil }

// SplitNames method
func (s SplitStorage) SplitNames() []string {
	toReturn := make([]string, 0)
	splits, _ := s.splitCollection.FetchAll()
	for _, split := range splits {
		toReturn = append(toReturn, split.Name)
	}
	return toReturn
}

// TrafficTypeExists method
func (s SplitStorage) TrafficTypeExists(trafficType string) bool { return false }
