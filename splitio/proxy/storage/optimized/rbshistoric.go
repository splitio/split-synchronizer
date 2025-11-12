package optimized

import (
	"sort"
	"sync"

	"github.com/splitio/go-split-commons/v8/dtos"
)

type HistoricChangesRB interface {
	GetUpdatedSince(since int64) []RBView
	Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, newCN int64)
}

type HistoricChangesRBImpl struct {
	data  []RBView
	mutex sync.RWMutex
}

func NewHistoricRBChanges(capacity int) *HistoricChangesRBImpl {
	return &HistoricChangesRBImpl{
		data: make([]RBView, 0, capacity),
	}
}

func (h *HistoricChangesRBImpl) GetUpdatedSince(since int64) []RBView {
	h.mutex.RLock()
	views := h.findNewerThan(since)
	toRet := copyAndFilterRB(views, since)
	h.mutex.RUnlock()
	return toRet
}

func (h *HistoricChangesRBImpl) Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, newCN int64) {
	h.mutex.Lock()
	h.updateFrom(toAdd)
	h.updateFrom(toRemove)
	sort.Slice(h.data, func(i, j int) bool { return h.data[i].LastUpdated < h.data[j].LastUpdated })
	h.mutex.Unlock()
}

// public interface ends here

func (h *HistoricChangesRBImpl) updateFrom(source []dtos.RuleBasedSegmentDTO) {
	for idx := range source {
		if current := h.findByName(source[idx].Name); current != nil {
			current.updateFrom(&source[idx])
		} else {
			var toAdd RBView
			toAdd.updateFrom(&source[idx])
			h.data = append(h.data, toAdd)
		}
	}
}

func (h *HistoricChangesRBImpl) findByName(name string) *RBView {
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

func (h *HistoricChangesRBImpl) findNewerThan(since int64) []RBView {
	// precondition: h.data is sorted by CN
	start := sort.Search(len(h.data), func(i int) bool { return h.data[i].LastUpdated > since })
	if start == len(h.data) {
		return nil
	}
	return h.data[start:]
}

type RBView struct {
	Name        string
	Active      bool
	LastUpdated int64
}

func (f *RBView) updateFrom(s *dtos.RuleBasedSegmentDTO) {
	f.Name = s.Name
	f.Active = s.Status == "ACTIVE"
	f.LastUpdated = s.ChangeNumber
}

func (f *RBView) clone() RBView {
	toRet := RBView{
		Name:        f.Name,
		Active:      f.Active,
		LastUpdated: f.LastUpdated,
	}
	return toRet

}

func copyAndFilterRB(views []RBView, since int64) []RBView {
	toRet := make([]RBView, 0, len(views))

	// this code computes the intersection in o(views * )
	for idx := range views {
		if featureShouldBeReturnedRB(&views[idx], since) {
			toRet = append(toRet, views[idx].clone())
		}
	}
	return toRet
}

func featureShouldBeReturnedRB(view *RBView, since int64) bool {
	// if fetching from sratch & the rule-based segment is not active,
	// or it hasn't been updated since `since`, it shouldn't even be considered for being returned
	if since == -1 && !view.Active || view.LastUpdated < since {
		return false
	}
	return true
}

var _ HistoricChangesRB = (*HistoricChangesRBImpl)(nil)
