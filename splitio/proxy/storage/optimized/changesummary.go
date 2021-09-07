package optimized

import (
	"errors"
	"sync"

	"github.com/splitio/go-split-commons/v4/dtos"
)

// ErrUnknownChangeNumber is returned when trying to fetch a recipe for a change number not present in cache
var ErrUnknownChangeNumber = errors.New("Unknown change number")

// SplitMinimalView is a subset of split properties needed by an sdk to remove a split from it's local cache
type SplitMinimalView struct {
	Name        string
	TrafficType string
}

// ChangeSummary represents a set of changes from/in a specific point in time
type ChangeSummary struct {
	Updated map[string]string // split name -> trafficType
	Removed map[string]string // split name -> trafficType
}

func newEmptyChangeSummary() ChangeSummary {
	return ChangeSummary{Updated: map[string]string{}, Removed: map[string]string{}}
}

func (c *ChangeSummary) applyChange(toAdd []SplitMinimalView, toRemove []SplitMinimalView) {
	for _, split := range toAdd {
		delete(c.Removed, split.Name)
		c.Updated[split.Name] = split.TrafficType
	}

	for _, split := range toRemove {
		if _, ok := c.Updated[split.Name]; ok {
			delete(c.Updated, split.Name)
		} else {
			c.Removed[split.Name] = split.TrafficType
		}
	}
}

// SplitChangesSummaries keeps a set of recipes that allow an sdk to fetch from any known changeNumber
// up to the latest snapshot.
type SplitChangesSummaries struct {
	currentCN int64
	changes   map[int64]ChangeSummary
	mutex     sync.RWMutex
}

// NewSplitChangesSummaries constructs a SplitChangesSummaries component
func NewSplitChangesSummaries() *SplitChangesSummaries {
	return &SplitChangesSummaries{
		currentCN: -1,
		changes:   map[int64]ChangeSummary{-1: newEmptyChangeSummary()},
	}
}

// AddChanges registers a new set of changes and updates all the recipes accordingly
func (s *SplitChangesSummaries) AddChanges(newCn int64, added []SplitMinimalView, removed []SplitMinimalView) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if newCn <= s.currentCN {
		return
	}

	for key, summary := range s.changes {
		summary.applyChange(added, removed)
		s.changes[key] = summary
	}

	s.currentCN = newCn
	s.changes[newCn] = newEmptyChangeSummary()
}

// AddOlderChange is used to add a change older than the oldest one currently stored (when the sync started)
// so that it can be used to serve SDKs stuck on an older CN
func (s *SplitChangesSummaries) AddOlderChange(cn int64, added []SplitMinimalView, removed []SplitMinimalView) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if cn >= s.currentCN {
		// If the change number is equal or greater than our current CN, we're about to overwrite
		// valid information, ignore it.
		return
	}

	summary := newEmptyChangeSummary()
	for _, split := range added {
		summary.Updated[split.Name] = split.TrafficType
	}

	for _, split := range removed {
		summary.Removed[split.Name] = split.TrafficType
	}

	s.changes[cn] = summary
}

// FetchSince returns a recipe explaining how to build a /splitChanges payload to serve an sdk which
// is currently on changeNumber `since`. It will contain the list of splits that need to be updated, and those that need
// to be deleted
func (s *SplitChangesSummaries) FetchSince(since int64) (*ChangeSummary, int64, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	view, ok := s.changes[since]
	if !ok {
		return nil, s.currentCN, ErrUnknownChangeNumber
	}
	return &view, s.currentCN, nil
}

// BuildArchivedSplitsFor takes a mapping of split name -> traffic type name,
// and build fake "ARCHIVED" splits to return to the sdk upon a splitChanges request
// now that we no longer store the full body of archived splits
func BuildArchivedSplitsFor(nameToTrafficType map[string]string) []dtos.SplitDTO {
	archived := make([]dtos.SplitDTO, 0, len(nameToTrafficType))
	for name, tt := range nameToTrafficType {
		archived = append(archived, dtos.SplitDTO{
			ChangeNumber:          1,
			TrafficTypeName:       tt,
			Name:                  name,
			TrafficAllocation:     100,
			TrafficAllocationSeed: 0,
			Seed:                  0,
			Status:                "ARCHIVED",
			Killed:                false,
			DefaultTreatment:      "off",
			Algo:                  1,
			Conditions:            make([]dtos.ConditionDTO, 0),
		})
	}
	return archived
}
