package caching

import (
	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/engine/grammar"
	"github.com/splitio/go-split-commons/v8/flagsets"
	"github.com/splitio/go-split-commons/v8/healthcheck/application"
	"github.com/splitio/go-split-commons/v8/service"
	"github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/largesegment"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v8/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/logging"
)

// CacheAwareSplitSynchronizer wraps a SplitSynchronizer and flushes cache when an update happens
type CacheAwareSplitSynchronizer struct {
	splitStorage storage.SplitStorage
	rbStorage    storage.RuleBasedSegmentsStorage
	wrapped      split.Updater
	cacheFlusher gincache.CacheFlusher
}

// NewCacheAwareSplitSync constructs a split-sync wrapper that evicts cache on updates
func NewCacheAwareSplitSync(
	splitStorage storage.SplitStorage,
	ruleBasedStorage storage.RuleBasedSegmentsStorage,
	splitFetcher service.SplitFetcher,
	logger logging.LoggerInterface,
	runtimeTelemetry storage.TelemetryRuntimeProducer,
	cacheFlusher gincache.CacheFlusher,
	appMonitor application.MonitorProducerInterface,
	flagSetsFilter flagsets.FlagSetFilter,
	specVersion string,
	ruleBuilder grammar.RuleBuilder,
) *CacheAwareSplitSynchronizer {
	return &CacheAwareSplitSynchronizer{
		wrapped:      split.NewSplitUpdater(splitStorage, ruleBasedStorage, splitFetcher, logger, runtimeTelemetry, appMonitor, flagSetsFilter, ruleBuilder, false, specVersion),
		splitStorage: splitStorage,
		rbStorage:    ruleBasedStorage,
		cacheFlusher: cacheFlusher,
	}
}

// SynchronizeSplits synchronizes feature flags and if something changes, purges the cache appropriately
func (c *CacheAwareSplitSynchronizer) SynchronizeSplits(till *int64) (*split.UpdateResult, error) {
	previous, _ := c.splitStorage.ChangeNumber()
	previousRB, _ := c.rbStorage.ChangeNumber()

	result, err := c.wrapped.SynchronizeSplits(till)
	current, _ := c.splitStorage.ChangeNumber()
	currentRB, _ := c.rbStorage.ChangeNumber()
	if current > previous || (previous != -1 && current == -1) || currentRB > previousRB || (previousRB != -1 && currentRB == -1) {
		// if the changenumber was updated, evict splitChanges responses from cache
		c.cacheFlusher.EvictBySurrogate(SplitSurrogate)
	}
	return result, err
}

// LocalKill kills a feature flag locally and purges splitChanges entries from the http cache
func (c *CacheAwareSplitSynchronizer) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	c.wrapped.LocalKill(splitName, defaultTreatment, changeNumber)
	// Since a feature flag was killed, unconditionally flush all feature flag changes
	c.cacheFlusher.EvictBySurrogate(SplitSurrogate)
}

// SynchronizeFeatureFlags synchronizes feature flags and if something changes, purges the cache appropriately
func (c *CacheAwareSplitSynchronizer) SynchronizeFeatureFlags(ffChange *dtos.SplitChangeUpdate) (*split.UpdateResult, error) {
	previous, _ := c.splitStorage.ChangeNumber()
	previousRB, _ := c.rbStorage.ChangeNumber()

	result, err := c.wrapped.SynchronizeFeatureFlags(ffChange)
	current, _ := c.splitStorage.ChangeNumber()
	currentRB, _ := c.rbStorage.ChangeNumber()
	if current > previous || (previous != -1 && current == -1) || currentRB > previousRB || (previousRB != -1 && currentRB == -1) {
		// if the changenumber was updated, evict splitChanges responses from cache
		c.cacheFlusher.EvictBySurrogate(SplitSurrogate)
	}
	return result, err
}

// CacheAwareSegmentSynchronizer wraps a segment-sync with cache-friendly logic
type CacheAwareSegmentSynchronizer struct {
	wrapped        segment.Updater
	splitStorage   storage.SplitStorage
	segmentStorage storage.SegmentStorage
	cacheFlusher   gincache.CacheFlusher
}

// NewCacheAwareSegmentSync constructs a new cache-aware segment sync
func NewCacheAwareSegmentSync(
	splitStorage storage.SplitStorage,
	segmentStorage storage.SegmentStorage,
	ruleBasedStorage storage.RuleBasedSegmentsStorage,
	segmentFetcher service.SegmentFetcher,
	logger logging.LoggerInterface,
	runtimeTelemetry storage.TelemetryRuntimeProducer,
	cacheFlusher gincache.CacheFlusher,
	appMonitor application.MonitorProducerInterface,
) *CacheAwareSegmentSynchronizer {
	return &CacheAwareSegmentSynchronizer{
		wrapped:        segment.NewSegmentUpdater(splitStorage, segmentStorage, ruleBasedStorage, segmentFetcher, logger, runtimeTelemetry, appMonitor),
		cacheFlusher:   cacheFlusher,
		splitStorage:   splitStorage,
		segmentStorage: segmentStorage,
	}
}

// SynchronizeSegment synchronizes a segment and if it was updated, flushes all entries associated with it from the http cache
func (c *CacheAwareSegmentSynchronizer) SynchronizeSegment(name string, till *int64) (*segment.UpdateResult, error) {
	previous, _ := c.segmentStorage.ChangeNumber(name)
	result, err := c.wrapped.SynchronizeSegment(name, till)
	if current := result.NewChangeNumber; current > previous || (previous != -1 && current == -1) {
		c.cacheFlusher.EvictBySurrogate(MakeSurrogateForSegmentChanges(name))
		c.cacheFlusher.EvictBySurrogate(MembershipsSurrogate)
	}

	// remove individual entries for each affected key
	for idx := range result.UpdatedKeys {
		for _, key := range MakeMySegmentsEntries(result.UpdatedKeys[idx]) {
			c.cacheFlusher.Evict(key)
		}
	}

	return result, err
}

// SynchronizeSegments syncs all the segments cached and purges cache appropriately if needed
func (c *CacheAwareSegmentSynchronizer) SynchronizeSegments() (map[string]segment.UpdateResult, error) {
	// we need to keep track of all change numbers here to see if anything changed
	previousSegmentNames := c.splitStorage.SegmentNames()
	previousCNs := make(map[string]int64, previousSegmentNames.Size())
	for _, name := range previousSegmentNames.List() {
		if strName, ok := name.(string); ok {
			cn, _ := c.segmentStorage.ChangeNumber(strName)
			previousCNs[strName] = cn
		}
	}

	results, err := c.wrapped.SynchronizeSegments()
	for segmentName := range results {
		result := results[segmentName]
		ccn := result.NewChangeNumber
		if pcn, _ := previousCNs[segmentName]; ccn > pcn || (pcn > 0 && ccn == -1) {
			// if the segment was updated or the segment was removed, evict it
			c.cacheFlusher.EvictBySurrogate(MakeSurrogateForSegmentChanges(segmentName))
			c.cacheFlusher.EvictBySurrogate(MembershipsSurrogate)
		}

		for idx := range result.UpdatedKeys {
			for _, key := range MakeMySegmentsEntries(result.UpdatedKeys[idx]) {
				c.cacheFlusher.Evict(key)
			}
		}

	}

	return results, err // return original segment sync error
}

// SegmentNames forwards the call to the wrapped sync
func (c *CacheAwareSegmentSynchronizer) SegmentNames() []interface{} {
	return c.wrapped.SegmentNames()
}

// IsSegmentCached forwards the call to the wrapped sync
func (c *CacheAwareSegmentSynchronizer) IsSegmentCached(segmentName string) bool {
	return c.wrapped.IsSegmentCached(segmentName)
}

// CacheAwareLargeSegmentSynchronizer
type CacheAwareLargeSegmentSynchronizer struct {
	wrapped             largesegment.Updater
	largeSegmentStorage storage.LargeSegmentsStorage
	cacheFlusher        gincache.CacheFlusher
	splitStorage        storage.SplitStorage
}

func NewCacheAwareLargeSegmentSync(
	splitStorage storage.SplitStorage,
	largeSegmentStorage storage.LargeSegmentsStorage,
	largeSegmentFetcher service.LargeSegmentFetcher,
	logger logging.LoggerInterface,
	runtimeTelemetry storage.TelemetryRuntimeProducer,
	cacheFlusher gincache.CacheFlusher,
	appMonitor application.MonitorProducerInterface,
) *CacheAwareLargeSegmentSynchronizer {
	return &CacheAwareLargeSegmentSynchronizer{
		wrapped:             largesegment.NewLargeSegmentUpdater(splitStorage, largeSegmentStorage, largeSegmentFetcher, logger, runtimeTelemetry, appMonitor),
		cacheFlusher:        cacheFlusher,
		largeSegmentStorage: largeSegmentStorage,
		splitStorage:        splitStorage,
	}
}

func (c *CacheAwareLargeSegmentSynchronizer) SynchronizeLargeSegment(name string, till *int64) (*int64, error) {
	previous := c.largeSegmentStorage.ChangeNumber(name)
	newCN, err := c.wrapped.SynchronizeLargeSegment(name, till)

	c.evictByLargeSegmentSurrogate(previous, *newCN)

	return newCN, err
}

func (c *CacheAwareLargeSegmentSynchronizer) SynchronizeLargeSegments() (map[string]*int64, error) {
	previousLargeSegmentNames := c.splitStorage.LargeSegmentNames()
	previousCNs := make(map[string]int64, previousLargeSegmentNames.Size())
	for _, name := range previousLargeSegmentNames.List() {
		if strName, ok := name.(string); ok {
			cn := c.largeSegmentStorage.ChangeNumber(strName)
			previousCNs[strName] = cn
		}
	}

	results, err := c.wrapped.SynchronizeLargeSegments()
	for name, currentCN := range results {
		c.evictByLargeSegmentSurrogate(previousCNs[name], *currentCN)
	}

	return results, err
}

func (c *CacheAwareLargeSegmentSynchronizer) IsCached(name string) bool {
	return c.wrapped.IsCached(name)
}

func (c *CacheAwareLargeSegmentSynchronizer) SynchronizeLargeSegmentUpdate(lsRFDResponseDTO *dtos.LargeSegmentRFDResponseDTO) (*int64, error) {
	previous := c.largeSegmentStorage.ChangeNumber(lsRFDResponseDTO.Name)
	newCN, err := c.wrapped.SynchronizeLargeSegmentUpdate(lsRFDResponseDTO)

	c.evictByLargeSegmentSurrogate(previous, *newCN)

	return newCN, err
}

func (c *CacheAwareLargeSegmentSynchronizer) evictByLargeSegmentSurrogate(previousCN int64, currentCN int64) {
	if currentCN > previousCN || currentCN == -1 {
		c.cacheFlusher.EvictBySurrogate(MembershipsSurrogate)
	}
}
