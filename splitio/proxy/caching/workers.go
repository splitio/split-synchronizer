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
) *CacheAwareSplitSynchronizer {
	return &CacheAwareSplitSynchronizer{
		wrapped:      split.NewSplitFetcher(splitStorage, splitFetcher, logger, runtimeTelemetry, &application.Dummy{}),
		splitStorage: splitStorage,
		cacheFlusher: cacheFlusher,
	}
}

// SynchronizeSplits synchronizes splits and if something changes, purges the cache appropriately
func (c *CacheAwareSplitSynchronizer) SynchronizeSplits(till *int64, requestNoCache bool) ([]string, error) {
	previous, _ := c.splitStorage.ChangeNumber()
	segmentList, err := c.wrapped.SynchronizeSplits(till, requestNoCache)
	if current, _ := c.splitStorage.ChangeNumber(); current > previous || (previous != -1 && current == -1) {
		// if the changenumber was updated, evict splitChanges responses from cache
		c.cacheFlusher.EvictBySurrogate(SplitSurrogate)
	}
	return segmentList, err
}

// LocalKill kills a split locally and purges splitChanges entries from the http cache
func (c *CacheAwareSplitSynchronizer) LocalKill(splitName string, defaultTreatment string, changeNumber int64) {
	c.wrapped.LocalKill(splitName, defaultTreatment, changeNumber)
	// Since a split was killed, unconditionally flush all split changes
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
) *CacheAwareSegmentSynchronizer {
	return &CacheAwareSegmentSynchronizer{
		wrapped:        segment.NewSegmentFetcher(splitStorage, segmentStorage, segmentFetcher, logger, runtimeTelemetry, &application.Dummy{}),
		cacheFlusher:   cacheFlusher,
		splitStorage:   splitStorage,
		segmentStorage: segmentStorage,
	}
}

// SynchronizeSegment synchronizes a segment and if it was updated, flushes all entries associated with it from the http cache
func (c *CacheAwareSegmentSynchronizer) SynchronizeSegment(name string, till *int64, requestNoCache bool) error {
	previous, _ := c.segmentStorage.ChangeNumber(name)
	err := c.wrapped.SynchronizeSegment(name, till, requestNoCache)
	if current, _ := c.segmentStorage.ChangeNumber(name); current > previous || (previous != -1 && current == -1) {
		c.cacheFlusher.EvictBySurrogate(MakeSurrogateForSegmentChanges(name))
	}

	return err
}

// SynchronizeSegments syncs all the segments cached and purges cache appropriately if needed
func (c *CacheAwareSegmentSynchronizer) SynchronizeSegments(requestNoCache bool) error {
	// we need to keep track of all change numbers here to see if anything changed
	previousSegmentNames := c.splitStorage.SegmentNames()
	previousCNs := make(map[string]int64, previousSegmentNames.Size())
	for _, name := range previousSegmentNames.List() {
		if strName, ok := name.(string); ok {
			cn, _ := c.segmentStorage.ChangeNumber(strName)
			previousCNs[strName] = cn
		}
	}

	err := c.wrapped.SynchronizeSegments(requestNoCache)

	currentSegmentNames := c.splitStorage.SegmentNames()
	currentCNs := make(map[string]int64, currentSegmentNames.Size())
	for _, name := range currentSegmentNames.List() {
		if strName, ok := name.(string); ok {
			cn, _ := c.segmentStorage.ChangeNumber(strName)
			currentCNs[strName] = cn
		}
	}

	// make a list of every updated segment
	toPurge := make(map[string]struct{})
	for name, pcn := range previousCNs { // add all removed & updated segments to the purge list
		if ccn, ok := currentCNs[name]; !ok || ccn > pcn || (pcn != -1 && ccn == -1) {
			toPurge[name] = struct{}{}
		}
	}

	for name := range currentCNs { // add any new segment (just in case we have some odd leftover)
		if _, ok := previousCNs[name]; !ok {
			toPurge[name] = struct{}{}
		}
	}

	for segmentName := range toPurge {
		c.cacheFlusher.EvictBySurrogate(MakeSurrogateForSegmentChanges(segmentName))
	}

	return err // return original segment sync error
}

// SegmentNames forwards the call to the wrapped sync
func (c *CacheAwareSegmentSynchronizer) SegmentNames() []interface{} {
	return c.wrapped.SegmentNames()
}

// IsSegmentCached forwards the call to the wrapped sync
func (c *CacheAwareSegmentSynchronizer) IsSegmentCached(segmentName string) bool {
	return c.wrapped.IsSegmentCached(segmentName)
}
