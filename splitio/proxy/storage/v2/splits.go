package v2

import (
	"errors"
	"sync"
)

var ErrUnknownChangeNumber = errors.New("Unknown change number")

type SplitMinimalView struct {
	Name        string
	TrafficType string
}

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

type SplitChangesSummaries struct {
	currentCN int64
	changes   map[int64]ChangeSummary
	mutex     sync.RWMutex
}

func NewSplitChangesSummaries() *SplitChangesSummaries {
	return &SplitChangesSummaries{
		currentCN: -1,
		changes:   map[int64]ChangeSummary{-1: newEmptyChangeSummary()},
	}
}

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

func (s *SplitChangesSummaries) FetchSince(since int64) (*ChangeSummary, int64, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	view, ok := s.changes[since]
	if !ok {
		return nil, s.currentCN, ErrUnknownChangeNumber
	}
	return &view, s.currentCN, nil
}
