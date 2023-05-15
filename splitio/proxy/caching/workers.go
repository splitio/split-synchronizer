package caching

import (
	"github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/split"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/gincache"
)

// CacheAwareSplitSynchronizer wraps a SplitSynchronizer and flushes cache when an update happens
type CacheAwareSplitSynchronizer struct {
	splitStorage storage.SplitStorage
	wrapped      split.Updater
	cacheFlusher gincache.CacheFlusher
}

// NewCacheAwareSplitSync constructs a split-sync wrapper that evicts cache on updates
func NewCacheAwareSplitSync(
	splitStorage storage.SplitStorage,
	splitFetcher service.SplitFetcher,
	logger logging.LoggerInterface,
	runtimeTelemetry storage.TelemetryRuntimeProducer,
	cacheFlusher gincache.CacheFlusher,
	appMonitor application.MonitorProducerInterface,
) *CacheAwareSplitSynchronizer {
	return &CacheAwareSplitSynchronizer{
		wrapped:      split.NewSplitFetcher(splitStorage, splitFetcher, logger, runtimeTelemetry, appMonitor),
		splitStorage: splitStorage,
		cacheFlusher: cacheFlusher,
	}
}

// SynchronizeSplits synchronizes feature flags and if something changes, purges the cache appropriately
func (c *CacheAwareSplitSynchronizer) SynchronizeSplits(till *int64) (*split.UpdateResult, error) {
	previous, _ := c.splitStorage.ChangeNumber()
	result, err := c.wrapped.SynchronizeSplits(till)
	if current, _ := c.splitStorage.ChangeNumber(); current > previous || (previous != -1 && current == -1) {
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
	segmentFetcher service.SegmentFetcher,
	logger logging.LoggerInterface,
	runtimeTelemetry storage.TelemetryRuntimeProducer,
	cacheFlusher gincache.CacheFlusher,
	appMonitor application.MonitorProducerInterface,
) *CacheAwareSegmentSynchronizer {
	return &CacheAwareSegmentSynchronizer{
		wrapped:        segment.NewSegmentFetcher(splitStorage, segmentStorage, segmentFetcher, logger, runtimeTelemetry, appMonitor),
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
	}

	// remove individual entries for each affected key
	for idx := range result.UpdatedKeys {
		c.cacheFlusher.Evict(MakeMySegmentsEntry(result.UpdatedKeys[idx]))
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
		}

		for idx := range result.UpdatedKeys {
			c.cacheFlusher.Evict(MakeMySegmentsEntry(result.UpdatedKeys[idx]))
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
