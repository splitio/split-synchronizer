package optimized

import (
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/splitio/go-split-commons/v5/dtos"
)

type HistoricChanges interface {
	GetUpdatedSince(since int64, flagSets []string) []FeatureView
	Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, newCN int64)
}

type HistoricChangesImpl struct {
	data  []FeatureView
	mutex sync.RWMutex
}

func NewHistoricSplitChanges(capacity int) *HistoricChangesImpl {
	return &HistoricChangesImpl{
		data: make([]FeatureView, 0, capacity),
	}
}

func (h *HistoricChangesImpl) GetUpdatedSince(since int64, flagSets []string) []FeatureView {
	slices.Sort(flagSets)
	h.mutex.RLock()
	views := h.findNewerThan(since)
	toRet := copyAndFilter(views, flagSets, since)
	h.mutex.RUnlock()
	return toRet
}

func (h *HistoricChangesImpl) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, newCN int64) {
	h.mutex.Lock()
	h.updateFrom(toAdd)
	h.updateFrom(toRemove)
	sort.Slice(h.data, func(i, j int) bool { return h.data[i].LastUpdated < h.data[j].LastUpdated })
	h.mutex.Unlock()
}

// public interface ends here

func (h *HistoricChangesImpl) updateFrom(source []dtos.SplitDTO) {
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

func (h *HistoricChangesImpl) findByName(name string) *FeatureView {
	// yes, it's linear search because features are sorted by CN, but it's only used
	// when processing an update coming from the BE. it's off the critical path of incoming
	// requests.
	for idx := range h.data {
		if h.data[idx].Name == name {
			return &h.data[idx]
		}
	}
	return nil
}

func (h *HistoricChangesImpl) findNewerThan(since int64) []FeatureView {
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

func (f *FeatureView) updateFlagsets(i []string, lastUpdated int64) {
	incoming := slices.Clone(i) // make a copy since we'll reorder elements

	// check if the current flagsets are still present in the updated split.
	// if they're present & currently marked as inactive, update their status & CN
	// if they're not present, mark them as ARCHIVED & update the CN
	for idx := range f.FlagSets {
		if itemIdx := slices.Index(incoming, f.FlagSets[idx].Name); itemIdx != -1 {
			if !f.FlagSets[idx].Active { // Association changed from ARCHIVED to ACTIVE
				f.FlagSets[idx].Active = true
				f.FlagSets[idx].LastUpdated = lastUpdated

			}

			// "soft delete" the item so that it's not traversed later on
			// (replaces the item with the last one and shrinks the slice by 1)
			incoming[itemIdx] = incoming[len(incoming)-1]
			incoming = incoming[:len(incoming)-1]

		} else { // Association changed from ARCHIVED to ACTIVE
			f.FlagSets[idx].Active = false
			f.FlagSets[idx].LastUpdated = lastUpdated
		}
	}

	// the only leftover in `incoming` should be the items that were not
	// present in the feature's previously associated flagsets, so they're new & active
	for idx := range incoming {
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

func (f *FeatureView) FlagSetNames() []string {
	toRet := make([]string, len(f.FlagSets))
	for idx := range f.FlagSets {
		toRet[idx] = f.FlagSets[idx].Name
	}
	return toRet
}

func copyAndFilter(views []FeatureView, sets []string, since int64) []FeatureView {
	// precondition: f.Flagsets is sorted by name
	// precondition: sets is sorted
	toRet := make([]FeatureView, 0, len(views))

	// this code computes the intersection in o(views * )
	for idx := range views {
		if featureShouldBeReturned(&views[idx], since, sets) {
			toRet = append(toRet, views[idx].clone())
		}
	}
	return toRet
}

func featureShouldBeReturned(view *FeatureView, since int64, sets []string) bool {

	// if fetching from sratch & the feature is not active,
	// or it hasn't been updated since `since`, it shouldn't even be considered for being returned
	if since == -1 && !view.Active || view.LastUpdated < since {
		return false
	}

	// all updated features should be returned if no set filter is being used
	if len(sets) == 0 {
		return true
	}

	// compare the sets for intersection of user supplied sets with currently active ones.
	// takes linear o(len(feature.sets) + len(sets)) time since both the incoming sets are sorted
	viewFlagSetIndex, requestedSetIndex := 0, 0
	for viewFlagSetIndex < len(view.FlagSets) {
		switch strings.Compare(view.FlagSets[viewFlagSetIndex].Name, sets[requestedSetIndex]) {
		case 0: // we got a match
			fsinfo := view.FlagSets[viewFlagSetIndex]
			// if an association is active, it's considered and the Feature is added to the result set.
			// if an association is inactive and we're fetching from scratch (since=-1), it's not considered.
			// if an association was already inactive at the time of the provided `since`, it's not considered.
			// if an association was active on the provided `since` and now isn't, the feature IS added to the returned payload.
			if fsinfo.Active || (since > -1 && since < fsinfo.LastUpdated) {
				return true
			}
			viewFlagSetIndex++
			incrUpTo(&requestedSetIndex, len(sets))
		case -1:
			viewFlagSetIndex++
		case 1:
			if incrUpTo(&requestedSetIndex, len(sets)); requestedSetIndex+1 == len(sets) {
				viewFlagSetIndex++
			}
		}
	}
	return false
}

type FlagSetView struct {
	Name        string
	Active      bool
	LastUpdated int64
}

func incrUpTo(toIncr *int, limit int) {
	if *toIncr+1 >= limit {
		return
	}
	*toIncr++
}

var _ HistoricChanges = (*HistoricChangesImpl)(nil)
