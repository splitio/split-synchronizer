package storage

import (
	"errors"
	"fmt"
	"sync"

	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v8/flagsets"
	"github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-split-commons/v8/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

// ErrSinceParamTooOld is returned when a summary is not cached for a requested change number
var ErrSinceParamTooOld = errors.New("summary for requested change number not cached")

// ProxySplitStorage defines the interface of a storage that can be used for serving splitChanges payloads
// for different requested `since` parameters
type ProxySplitStorage interface {
	ChangesSince(since int64, flagSets []string) (*dtos.SplitChangesDTO, error)
}

// ProxySplitStorageImpl implements the ProxySplitStorage interface and the SplitProducer interface
type ProxySplitStorageImpl struct {
	snapshot      mutexmap.MMSplitStorage
	db            *persistent.SplitChangesCollection
	flagSets      flagsets.FlagSetFilter
	historic      optimized.HistoricChanges
	logger        logging.LoggerInterface
	oldestKnownCN int64
	mtx           sync.Mutex
}

// NewProxySplitStorage instantiates a new proxy storage that wraps an in-memory snapshot of the last known,
// flag configuration, a changes summaries containing recipes to update SDKs with different CNs, and a persistent storage
// for snapshot purposes
func NewProxySplitStorage(db persistent.DBWrapper, logger logging.LoggerInterface, flagSets flagsets.FlagSetFilter, restoreBackup bool) *ProxySplitStorageImpl {
	disk := persistent.NewSplitChangesCollection(db, logger)
	snapshot := mutexmap.NewMMSplitStorage(flagSets)
	historic := optimized.NewHistoricSplitChanges(1000)

	var initialCN int64 = -1
	if restoreBackup {
		initialCN = snapshotFromDisk(snapshot, historic, disk, logger)
	}
	return &ProxySplitStorageImpl{
		snapshot:      *snapshot,
		db:            disk,
		flagSets:      flagSets,
		historic:      historic,
		logger:        logger,
		oldestKnownCN: initialCN,
	}
}

// ChangesSince builds a SplitChanges payload to from `since` to the latest known CN
func (p *ProxySplitStorageImpl) ChangesSince(since int64, flagSets []string) (*dtos.SplitChangesDTO, error) {

	// No flagsets and fetching from -1, return the current snapshot
	if since == -1 && len(flagSets) == 0 {
		cn, err := p.snapshot.ChangeNumber()
		if err != nil {
			return nil, fmt.Errorf("error fetching changeNumber from snapshot: %w", err)
		}
		all := p.snapshot.All()
		return &dtos.SplitChangesDTO{Since: since, Till: cn, Splits: all}, nil
	}

	if p.sinceIsTooOld(since) {
		return nil, ErrSinceParamTooOld
	}

	views := p.historic.GetUpdatedSince(since, flagSets)
	namesToFetch := make([]string, 0, len(views))
	all := make([]dtos.SplitDTO, 0, len(views))
	var till int64 = since
	for idx := range views {
		if t := views[idx].LastUpdated; t > till {
			till = t
		}
		if views[idx].Active {
			namesToFetch = append(namesToFetch, views[idx].Name)
		} else {
			all = append(all, archivedDTOForView(&views[idx]))
		}
	}

	for name, split := range p.snapshot.FetchMany(namesToFetch) {
		if split == nil {
			p.logger.Warning(fmt.Sprintf(
				"possible inconsistency between historic & snapshot storages. Feature `%s` is missing in the latter",
				name,
			))
			continue
		}
		all = append(all, *split)
	}

	return &dtos.SplitChangesDTO{Since: since, Till: till, Splits: all}, nil
}

// KillLocally marks a feature flag as killed in the current storage
func (p *ProxySplitStorageImpl) KillLocally(splitName string, defaultTreatment string, changeNumber int64) {
	p.snapshot.KillLocally(splitName, defaultTreatment, changeNumber)
}

// Update the storage atomically
func (p *ProxySplitStorageImpl) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, changeNumber int64) {

	p.setStartingPoint(changeNumber) // will be executed only the first time this method is called

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}

	p.mtx.Lock()
	p.snapshot.Update(toAdd, toRemove, changeNumber)
	p.historic.Update(toAdd, toRemove, changeNumber)
	p.db.Update(toAdd, toRemove, changeNumber)
	p.mtx.Unlock()
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

// LargeSegmentNames call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) LargeSegmentNames() *set.ThreadUnsafeSet {
	return p.snapshot.LargeSegmentNames()
}

// Split call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) Split(name string) *dtos.SplitDTO { return p.snapshot.Split(name) }

// SplitNames call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) SplitNames() []string { return p.snapshot.SplitNames() }

// TrafficTypeExists call is forwarded to the snapshot
func (p *ProxySplitStorageImpl) TrafficTypeExists(tt string) bool {
	return p.snapshot.TrafficTypeExists(tt)
}

// Count returns the number of cached feature flags
func (p *ProxySplitStorageImpl) Count() int {
	return len(p.SplitNames())
}

// GetNamesByFlagSets implements storage.SplitStorage
func (p *ProxySplitStorageImpl) GetNamesByFlagSets(sets []string) map[string][]string {
	return p.snapshot.GetNamesByFlagSets(sets)
}

// GetAllFlagSetNames implements storage.SplitStorage
func (p *ProxySplitStorageImpl) GetAllFlagSetNames() []string {
	return p.snapshot.GetAllFlagSetNames()
}

func (p *ProxySplitStorageImpl) setStartingPoint(cn int64) {
	p.mtx.Lock()
	// will be executed only the first time this method is called or when
	// an older change is registered
	if p.oldestKnownCN == -1 || cn < p.oldestKnownCN {
		p.oldestKnownCN = cn
	}
	p.mtx.Unlock()
}

func (p *ProxySplitStorageImpl) ReplaceAll(splits []dtos.SplitDTO, changeNumber int64) error {
	panic("not implemented")
}

func (p *ProxySplitStorageImpl) RuleBasedSegmentNames() *set.ThreadUnsafeSet {
	return p.snapshot.RuleBasedSegmentNames()
}

func (p *ProxySplitStorageImpl) sinceIsTooOld(since int64) bool {
	if since == -1 {
		return false
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()
	return since < p.oldestKnownCN
}

func snapshotFromDisk(
	dst *mutexmap.MMSplitStorage,
	historic optimized.HistoricChanges,
	src *persistent.SplitChangesCollection,
	logger logging.LoggerInterface,
) int64 {
	all, err := src.FetchAll()
	if err != nil {
		logger.Error("error parsing feature flags from snapshot. No data will be available!: ", err)
		return -1
	}

	var filtered []dtos.SplitDTO
	var cn = src.ChangeNumber()
	for idx := range all {

		// Make sure the CN matches is at least large as the payloads' max.
		if thisCN := all[idx].ChangeNumber; thisCN > cn {
			cn = thisCN
		}
		if all[idx].Status == "ACTIVE" {
			filtered = append(filtered, all[idx])
		}
	}

	dst.Update(filtered, nil, cn)
	historic.Update(filtered, nil, cn)
	return cn
}

func archivedDTOForView(view *optimized.FeatureView) dtos.SplitDTO {
	return dtos.SplitDTO{
		ChangeNumber:          view.LastUpdated,
		TrafficTypeName:       view.TrafficTypeName,
		Name:                  view.Name,
		TrafficAllocation:     100,
		TrafficAllocationSeed: 0,
		Seed:                  0,
		Status:                constants.SplitStatusArchived,
		Killed:                false,
		DefaultTreatment:      "off",
		Algo:                  1,
		Conditions:            make([]dtos.ConditionDTO, 0),
		Sets:                  view.FlagSetNames(),
	}
}

var _ ProxySplitStorage = (*ProxySplitStorageImpl)(nil)
var _ storage.SplitStorage = (*ProxySplitStorageImpl)(nil)
var _ observability.ObservableSplitStorage = (*ProxySplitStorageImpl)(nil)
