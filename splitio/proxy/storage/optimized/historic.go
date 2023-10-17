package optimized

import (
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/splitio/go-split-commons/v5/dtos"
)

type HistoricChanges struct {
	data  []FeatureView
	mutex sync.RWMutex
}

func (h *HistoricChanges) GetUpdatedSince(since int64, flagSets []string) []FeatureView {
	h.mutex.RLock()
	views := h.findNewerThan(since)
	toRet := copyAndFilter(views, flagSets, since)
	h.mutex.RUnlock()
	return toRet
}

func (h *HistoricChanges) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, newCN int64) {
	h.mutex.Lock()
	h.updateFrom(toAdd)
	h.updateFrom(toRemove)
	sort.Slice(h.data, func(i, j int) bool { return h.data[i].LastUpdated < h.data[j].LastUpdated })
	h.mutex.Unlock()
}

func (h *HistoricChanges) updateFrom(source []dtos.SplitDTO) {
	for idx := range source {
		if current := h.findByName(source[idx].Name); current != nil {
			current.updateFrom(&source[idx])
		} else {
			var toAdd FeatureView
			toAdd.updateFrom(&source[idx])
			h.data = append(h.data, toAdd)
		}
	}

}

func (h *HistoricChanges) findByName(name string) *FeatureView {
	for idx := range h.data {
		if h.data[idx].Name == name { // TODO(mredolatti): optimize!
			return &h.data[idx]
		}
	}
	return nil
}

func (h *HistoricChanges) findNewerThan(since int64) []FeatureView {
	// precondition: h.data is sorted by CN
	start := sort.Search(len(h.data), func(i int) bool { return h.data[i].LastUpdated > since })
	if start == len(h.data) {
		return nil
	}
	return h.data[start:]
}

type FeatureView struct {
	Name            string
	Active          bool
	LastUpdated     int64
	TrafficTypeName string
	FlagSets        []FlagSetView
}

func (f *FeatureView) updateFrom(s *dtos.SplitDTO) {
	f.Name = s.Name
	f.Active = s.Status == "ACTIVE"
	f.LastUpdated = s.ChangeNumber
	f.TrafficTypeName = s.TrafficTypeName
	f.updateFlagsets(s.Sets, s.ChangeNumber)
}

func (f *FeatureView) updateFlagsets(incoming []string, lastUpdated int64) {
	// TODO(mredolatti): need a copy of incoming?
	for idx := range f.FlagSets {
		if itemIdx := slices.Index(incoming, f.FlagSets[idx].Name); itemIdx != -1 {
			if !f.FlagSets[idx].Active { // Association changed from ARCHIVED to ACTIVE
				f.FlagSets[idx].Active = true
				f.FlagSets[idx].LastUpdated = lastUpdated

			}

			// "soft delete" the item so that it's not traversed later on
			// (replaces the item with the last one, clears the latter and shrinks the slice by 1)
			incoming[itemIdx] = incoming[len(incoming)-1]
			incoming[len(incoming)-1] = ""
			incoming = incoming[:len(incoming)-1]

		} else { // Association changed from ARCHIVED to ACTIVE
			f.FlagSets[idx].Active = false
			f.FlagSets[idx].LastUpdated = lastUpdated
		}
	}

	for idx := range incoming {
		// the only leftover in `incoming` should be the items that were not
		// present in the feature's previously associated flagsets, so they're new & active
		f.FlagSets = append(f.FlagSets, FlagSetView{
			Name:        incoming[idx],
			Active:      true,
			LastUpdated: lastUpdated,
		})
	}

	sort.Slice(f.FlagSets, func(i, j int) bool { return f.FlagSets[i].Name < f.FlagSets[j].Name })
}

func (f *FeatureView) findFlagSetByName(name string) *FlagSetView {
	// precondition: f.FlagSets is sorted by name
	idx := sort.Search(len(f.FlagSets), func(i int) bool { return f.FlagSets[i].Name >= name })
	if idx != len(f.FlagSets) && name == f.FlagSets[idx].Name {
		return &f.FlagSets[idx]
	}
	return nil
}

func (f *FeatureView) clone() FeatureView {
	toRet := FeatureView{
		Name:            f.Name,
		Active:          f.Active,
		LastUpdated:     f.LastUpdated,
		TrafficTypeName: f.TrafficTypeName,
		FlagSets:        make([]FlagSetView, len(f.FlagSets)),
	}
	copy(toRet.FlagSets, f.FlagSets) // we need to deep clone to avoid race conditions
	return toRet

}

func copyAndFilter(views []FeatureView, sets []string, since int64) []FeatureView {
	// precondition: f.Flagsets is sorted by name
	// precondition: sets is sorted
	toRet := make([]FeatureView, 0, len(views))
	if len(sets) == 0 {
		for idx := range views {
			toRet = append(toRet, views[idx].clone())
		}
		return toRet
	}

	// this code computes the intersection in o(views * (len(views.sets) + len(sets)))
	for idx := range views {
		viewFlagSetIndex, requestedSetIndex := 0, 0
		for viewFlagSetIndex < len(views[idx].FlagSets) {
			switch strings.Compare(views[idx].FlagSets[viewFlagSetIndex].Name, sets[requestedSetIndex]) {
			case 0: // we got a match
				fsinfo := views[idx].FlagSets[viewFlagSetIndex]
				// if an association is active, it's considered and the Feature is added to the result set.
				// if an association is inactive and we're fetching from scratch (since=-1), it's not considered.
				// if an association was already inactive at the time of the provided `since`, it's not considered.
				// if an association was active on the provided `since` and now isn't, the feature IS added to the returned payload.
				//  - the consumer is responsible for filtering flagsets where active = false when mapping the outcome of
				//    this function to a []dtos.SplitChanges response.
				if fsinfo.Active || (since > -1 && fsinfo.LastUpdated > since) {
					toRet = append(toRet, views[idx].clone())
				}
				viewFlagSetIndex++
				incrUpTo(&requestedSetIndex, len(sets))
			case -1:
				viewFlagSetIndex++
			case 1:
				if incrUpTo(&requestedSetIndex, len(sets)) {
					viewFlagSetIndex++
				}
			}
		}
	}
	return toRet
}

type FlagSetView struct {
	Name        string
	Active      bool
	LastUpdated int64
}

// increment `toIncr` by 1 as long as the result is less than `limit`.
// return wether the limit was reached
func incrUpTo(toIncr *int, limit int) bool {
	if *toIncr+1 >= limit {
		return true
	}
	*toIncr++
	return false
}
