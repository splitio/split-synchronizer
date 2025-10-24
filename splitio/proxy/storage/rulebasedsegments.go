package storage

import (
	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-split-commons/v8/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

// ProxyRuleBasedSegmentsStorage defines the interface of a storage that can be used for serving payloads
// for different requested `since` parameters
type ProxyRuleBasedSegmentsStorage interface {
	ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error)
}

// ProxyRuleBasedSegmentsStorageImpl implements the ProxyRuleBasedSegmentsStorage interface and the SplitProducer interface
type ProxyRuleBasedSegmentsStorageImpl struct {
	snapshot mutexmap.RuleBasedSegmentsStorageImpl
	logger   logging.LoggerInterface
	// mtx      sync.Mutex
}

// NewProxyRuleBasedSegmentsStorage instantiates a new proxy storage that wraps an in-memory snapshot of the last known
// flag configuration
func NewProxyRuleBasedSegmentsStorage(logger logging.LoggerInterface) *ProxyRuleBasedSegmentsStorageImpl {
	snapshot := mutexmap.NewRuleBasedSegmentsStorage()

	return &ProxyRuleBasedSegmentsStorageImpl{
		snapshot: *snapshot,
		logger:   logger,
	}
}

// ChangesSince retrieves the rule-based segments changes since the given change number
func (p *ProxyRuleBasedSegmentsStorageImpl) ChangesSince(since int64) (*dtos.RuleBasedSegmentsDTO, error) {
	cn, _ := p.snapshot.ChangeNumber()
	return &dtos.RuleBasedSegmentsDTO{Since: since, Till: cn, RuleBasedSegments: p.snapshot.All()}, nil
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
func (p *ProxyRuleBasedSegmentsStorageImpl) Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, cn int64) error {
	// TODO Add the other logic
	// 	p.setStartingPoint(changeNumber) // will be executed only the first time this method is called

	// if len(toAdd) == 0 && len(toRemove) == 0 {
	// 	return
	// }

	// p.mtx.Lock()
	// p.snapshot.Update(toAdd, toRemove, changeNumber)
	// p.historic.Update(toAdd, toRemove, changeNumber)
	// p.db.Update(toAdd, toRemove, changeNumber)
	// p.mtx.Unlock()

	p.snapshot.Update(toAdd, toRemove, cn)
	return nil
}

var _ ProxyRuleBasedSegmentsStorage = (*ProxyRuleBasedSegmentsStorageImpl)(nil)
var _ storage.RuleBasedSegmentsStorage = (*ProxyRuleBasedSegmentsStorageImpl)(nil)
