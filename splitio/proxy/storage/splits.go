package storage

import (
	"errors"
	"fmt"
	"sync"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/persistent"
)

// ErrSummaryNotCached is returned when a summary is not cached for a requested change number
var ErrSummaryNotCached = errors.New("summary for requested change number not cached")

// ProxySplitStorage defines the interface of a storage that can be used for serving splitChanges payloads
// for different requested `since` parameters
type ProxySplitStorage interface {
	ChangesSince(since int64) (*dtos.SplitChangesDTO, error)
	RegisterOlderCn(payload *dtos.SplitChangesDTO)
}

// ProxySplitStorageImpl implements the ProxySplitStorage interface and the SplitProducer interface
type ProxySplitStorageImpl struct {
	snapshot mutexmap.MMSplitStorage
	recipes  optimized.SplitChangesSummaries
	db       *persistent.SplitChangesCollection
	mtx      sync.Mutex
}

// NewProxySplitStorage instantiates a new proxy storage that wraps an in-memory snapshot of the last known,
// flag configuration, a changes summaries containing recipes to update SDKs with different CNs, and a persistent storage
// for snapshot purposes
func NewProxySplitStorage(db persistent.DBWrapper, logger logging.LoggerInterface) *ProxySplitStorageImpl {
	return &ProxySplitStorageImpl{
		snapshot: *mutexmap.NewMMSplitStorage(),
		recipes:  *optimized.NewSplitChangesSummaries(),
		db:       persistent.NewSplitChangesCollection(db, logger),
	}
}

// ChangesSince builds a SplitChanges payload to from `since` to the latest known CN
func (p *ProxySplitStorageImpl) ChangesSince(since int64) (*dtos.SplitChangesDTO, error) {
	// Special case of -1, return all
	if since == -1 {
		cn, err := p.snapshot.ChangeNumber()
		if err != nil {
			return nil, fmt.Errorf("error fetching changeNumber from snapshot: %w", err)
		}
		all := p.snapshot.All()
		return &dtos.SplitChangesDTO{Since: since, Till: cn, Splits: all}, nil
	}

	summary, till, err := p.recipes.FetchSince(int64(since))
	if err != nil {
		if errors.Is(err, ErrSummaryNotCached) {
			return nil, ErrSummaryNotCached
		}
		return nil, fmt.Errorf("unexpected error when fetching changes summary: %w", err)
	}

	// Regular flow
	splitNames := make([]string, 0, len(summary.Updated))
	for name := range summary.Updated {
		splitNames = append(splitNames, name)
	}

	active := p.snapshot.FetchMany(splitNames)
	all := make([]dtos.SplitDTO, 0, len(summary.Removed)+len(summary.Updated))
	for _, split := range active {
		all = append(all, *split)
	}
	all = append(all, optimized.BuildArchivedSplitsFor(summary.Removed)...)
	return &dtos.SplitChangesDTO{Since: since, Till: till, Splits: all}, nil
}

// KillLocally marks a split as killed in the current storage
func (p *ProxySplitStorageImpl) KillLocally(splitName string, defaultTreatment string, changeNumber int64) {
	p.snapshot.KillLocally(splitName, defaultTreatment, changeNumber)
}

// Update the storage atomically
func (p *ProxySplitStorageImpl) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) {

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}

	p.mtx.Lock()
	p.snapshot.Update(toAdd, toRemove, changeNumber)
	p.recipes.AddChanges(toAdd, toRemove, changeNumber)
	p.db.Update(toAdd, toRemove, changeNumber)
	p.mtx.Unlock()
}

// RegisterOlderCn registers payload associated to a fetch request for an old `since` for which we don't
// have a recipe
func (p *ProxySplitStorageImpl) RegisterOlderCn(payload *dtos.SplitChangesDTO) {
	toAdd := make([]dtos.SplitDTO, 0)
	toDel := make([]dtos.SplitDTO, 0)
	for _, split := range payload.Splits {
		if split.Status == "ACTIVE" {
			toAdd = append(toAdd, split)
		} else {
			toDel = append(toDel, split)
		}
	}
	p.recipes.AddOlderChange(toAdd, toDel, payload.Till)
}

// ChangeNumber returns the current change number
func (p *ProxySplitStorageImpl) ChangeNumber() (int64, error) {
	return p.snapshot.ChangeNumber()
}

// SetChangeNumber updates the change number
func (p *ProxySplitStorageImpl) SetChangeNumber(cn int64) error {
	return p.snapshot.SetChangeNumber(cn)
}

// Remove deletes a split by name
func (p *ProxySplitStorageImpl) Remove(name string) {
	p.snapshot.Remove(name)
}

// All call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) All() []dtos.SplitDTO { return p.snapshot.All() }

// FetchMany call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) FetchMany(names []string) map[string]*dtos.SplitDTO {
	return p.snapshot.FetchMany(names)
}

// SegmentNames call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) SegmentNames() *set.ThreadUnsafeSet { return p.snapshot.SegmentNames() }

// Split call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) Split(name string) *dtos.SplitDTO { return p.snapshot.Split(name) }

// SplitNames call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) SplitNames() []string { return p.snapshot.SplitNames() }

// TrafficTypeExists call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) TrafficTypeExists(tt string) bool {
	return p.snapshot.TrafficTypeExists(tt)
}

var _ ProxySplitStorage = (*ProxySplitStorageImpl)(nil)
var _ storage.SplitStorage = (*ProxySplitStorageImpl)(nil)
