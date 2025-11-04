package storage

import (
	"fmt"
	"sync"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-split-commons/v8/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
)

// ProxyRuleBasedSegmentsStorage defines the interface of a storage that can be used for serving payloads
// for different requested `since` parameters
type ProxyRuleBasedSegmentsStorage interface {
	ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error)
}

// ProxyRuleBasedSegmentsStorageImpl implements the ProxyRuleBasedSegmentsStorage interface and the RuleBasedSegmentProducer interface
type ProxyRuleBasedSegmentsStorageImpl struct {
	snapshot      mutexmap.RuleBasedSegmentsStorageImpl
	logger        logging.LoggerInterface
	oldestKnownCN int64
	mtx           sync.Mutex
	historic      optimized.HistoricChangesRB
}

// NewProxyRuleBasedSegmentsStorage instantiates a new proxy storage that wraps an in-memory snapshot of the last known
// flag configuration
func NewProxyRuleBasedSegmentsStorage(logger logging.LoggerInterface) *ProxyRuleBasedSegmentsStorageImpl {
	snapshot := mutexmap.NewRuleBasedSegmentsStorage()
	historic := optimized.NewHistoricRBChanges(1000)
	var initialCN int64 = -1

	return &ProxyRuleBasedSegmentsStorageImpl{
		snapshot:      *snapshot,
		logger:        logger,
		oldestKnownCN: initialCN,
		historic:      historic,
	}
}

func (p *ProxyRuleBasedSegmentsStorageImpl) sinceIsTooOld(since int64) bool {
	if since == -1 {
		return false
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()
	return since < p.oldestKnownCN
}

func archivedRBDTOForView(view *optimized.RBView) dtos.RuleBasedSegmentDTO {
	return dtos.RuleBasedSegmentDTO{
		ChangeNumber: view.LastUpdated,
		Name:         view.Name,
		Status:       constants.SplitStatusArchived,
	}
}

// ChangesSince retrieves the rule-based segments changes since the given change number
func (p *ProxyRuleBasedSegmentsStorageImpl) ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error) {
	// No flagsets and fetching from -1, return the current snapshot
	if since == -1 {
		cn, err := p.snapshot.ChangeNumber()
		if err != nil {
			return nil, fmt.Errorf("error fetching changeNumber from snapshot: %w", err)
		}
		all := p.snapshot.All()
		return &dtos.RuleBasedSegmentsDTO{Since: since, Till: cn, RuleBasedSegments: all}, nil
	}

	if p.sinceIsTooOld(since) {
		return nil, ErrSinceParamTooOld
	}

	views := p.historic.GetUpdatedSince(since)
	namesToFetch := make([]string, 0, len(views))
	all := make([]dtos.RuleBasedSegmentDTO, 0, len(views))
	var till int64 = since
	for idx := range views {
		if t := views[idx].LastUpdated; t > till {
			till = t
		}
		if views[idx].Active {
			namesToFetch = append(namesToFetch, views[idx].Name)
		} else {
			all = append(all, archivedRBDTOForView(&views[idx]))
		}
	}

	for name, rbSegments := range p.snapshot.FetchMany(namesToFetch) {
		if rbSegments == nil {
			p.logger.Warning(fmt.Sprintf(
				"possible inconsistency between historic & snapshot storages. Rule-based segment `%s` is missing in the latter",
				name,
			))
			continue
		}
		all = append(all, *rbSegments)
	}
	return &dtos.RuleBasedSegmentsDTO{Since: since, Till: till, RuleBasedSegments: all}, nil

	// if since > -1 {
	// 	return &dtos.RuleBasedSegmentsDTO{Since: since, Till: since, RuleBasedSegments: []dtos.RuleBasedSegmentDTO{}}, nil
	// }
	// cn, _ := p.snapshot.ChangeNumber()
	// return &dtos.RuleBasedSegmentsDTO{Since: since, Till: cn, RuleBasedSegments: p.snapshot.All()}, nil
}

// All call is forwarded to the snapshot
func (p *ProxyRuleBasedSegmentsStorageImpl) All() []dtos.RuleBasedSegmentDTO {
	return p.snapshot.All()
}

// ChangeNumber returns the current change number
func (p *ProxyRuleBasedSegmentsStorageImpl) ChangeNumber() (int64, error) {
	return p.snapshot.ChangeNumber()
}

// Contains checks if the given rule-based segments are present in storage
func (p *ProxyRuleBasedSegmentsStorageImpl) Contains(rbs []string) bool {
	return p.snapshot.Contains(rbs)
}

// GetRuleBasedSegmentByName retrieves a rule-based segment by name
func (p *ProxyRuleBasedSegmentsStorageImpl) GetRuleBasedSegmentByName(name string) (*dtos.RuleBasedSegmentDTO, error) {
	return p.snapshot.GetRuleBasedSegmentByName(name)
}

// LargeSegments call is forwarded to the snapshot
func (p *ProxyRuleBasedSegmentsStorageImpl) LargeSegments() *set.ThreadUnsafeSet {
	return p.snapshot.LargeSegments()
}

// ReplaceAll replaces all rule-based segments in storage
func (p *ProxyRuleBasedSegmentsStorageImpl) ReplaceAll(rbs []dtos.RuleBasedSegmentDTO, cn int64) error {
	return p.snapshot.ReplaceAll(rbs, cn)
}

// RuleBasedSegmentNames retrieves the names of all rule-based segments
func (p *ProxyRuleBasedSegmentsStorageImpl) RuleBasedSegmentNames() ([]string, error) {
	return p.snapshot.RuleBasedSegmentNames()
}

// Segments retrieves the names of all segments used in rule-based segments
func (p *ProxyRuleBasedSegmentsStorageImpl) Segments() *set.ThreadUnsafeSet {
	return p.snapshot.Segments()
}

// SetChangeNumber sets the change number
func (p *ProxyRuleBasedSegmentsStorageImpl) SetChangeNumber(cn int64) error {
	return p.snapshot.SetChangeNumber(cn)
}

// Update
func (p *ProxyRuleBasedSegmentsStorageImpl) Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, changeNumber int64) error {
	// TODO Add the other logic
	p.setStartingPoint(changeNumber) // will be executed only the first time this method is called

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil
	}

	p.mtx.Lock()
	p.snapshot.Update(toAdd, toRemove, changeNumber)
	p.historic.Update(toAdd, toRemove, changeNumber)
	// p.db.Update(toAdd, toRemove, changeNumber)
	p.mtx.Unlock()
	return nil
}

func (p *ProxyRuleBasedSegmentsStorageImpl) setStartingPoint(cn int64) {
	p.mtx.Lock()
	// will be executed only the first time this method is called or when
	// an older change is registered
	if p.oldestKnownCN == -1 || cn < p.oldestKnownCN {
		p.oldestKnownCN = cn
	}
	p.mtx.Unlock()
}

// FetchMany fetches rule-based segments in the storage and returns an array of rule-based segments dtos
func (p *ProxyRuleBasedSegmentsStorageImpl) FetchMany(rbsNames []string) map[string]*dtos.RuleBasedSegmentDTO {
	return p.snapshot.FetchMany(rbsNames)
}

var _ ProxyRuleBasedSegmentsStorage = (*ProxyRuleBasedSegmentsStorageImpl)(nil)
var _ storage.RuleBasedSegmentsStorage = (*ProxyRuleBasedSegmentsStorageImpl)(nil)
