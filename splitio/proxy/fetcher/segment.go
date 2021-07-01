package fetcher

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-split-commons/v3/service"
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-split-commons/v3/storage/mutexmap"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v3/util"
	"github.com/splitio/go-toolkit/v4/datastructures/set"
	"github.com/splitio/go-toolkit/v4/logging"
)

// SegmentFetcherProxy struct
type SegmentFetcherProxy struct {
	segmentStorage *mutexmap.MMSegmentStorage
	mySegments     *MySegmentsCache
	splitStorage   storage.SplitStorageConsumer
	segmentFetcher service.SegmentFetcher
	metricsWrapper *storage.MetricWrapper
	logger         logging.LoggerInterface
}

// NewSegmentFetcher build new fetcher for proxy
func NewSegmentFetcher(
	segmentStorage *mutexmap.MMSegmentStorage,
	splitStorage storage.SplitStorageConsumer,
	segmentFetcher service.SegmentFetcher,
	metricsWrapper *storage.MetricWrapper,
	logger logging.LoggerInterface,
	mySegmentsCache *MySegmentsCache,
) segment.Updater {
	return &SegmentFetcherProxy{
		segmentStorage: segmentStorage,
		mySegments:     mySegmentsCache,
		splitStorage:   splitStorage,
		segmentFetcher: segmentFetcher,
		metricsWrapper: metricsWrapper,
		logger:         logger,
	}
}

// SegmentNames segmentNames
func (s *SegmentFetcherProxy) SegmentNames() []interface{} {
	return s.splitStorage.SegmentNames().List()
}

// SynchronizeSegments syncs segments
func (s *SegmentFetcherProxy) SynchronizeSegments(requestNoCache bool) error {
	segmentNames := s.splitStorage.SegmentNames().List()
	s.logger.Debug("Segment Sync", segmentNames)
	wg := sync.WaitGroup{}
	wg.Add(len(segmentNames))
	failedSegments := set.NewThreadSafeSet()
	for _, name := range segmentNames {
		conv, ok := name.(string)
		if !ok {
			s.logger.Warning("Skipping non-string segment present in storage at initialization-time!")
			continue
		}
		go func(segmentName string) {
			defer wg.Done() // Make sure the "finished" signal is always sent
			ready := false
			var err error
			for !ready {
				err = s.SynchronizeSegment(segmentName, nil, requestNoCache)
				if err != nil {
					failedSegments.Add(segmentName)
				}
				return
			}
		}(conv)
	}
	wg.Wait()

	if failedSegments.Size() > 0 {
		return fmt.Errorf("The following segments failed to be fetched %v", failedSegments.List())
	}

	return nil
}

func (s *SegmentFetcherProxy) processUpdate(segmentChanges *dtos.SegmentChangesDTO) {
	name := segmentChanges.Name
	oldSegment := s.segmentStorage.Keys(name)
	if oldSegment == nil {
		keys := set.NewSet()
		for _, key := range segmentChanges.Added {
			keys.Add(key)
		}
		s.logger.Debug(fmt.Sprintf("Segment [%s] doesn't exist now, it will add (%d) keys", name, keys.Size()))
		s.segmentStorage.Update(name, keys, set.NewSet(), segmentChanges.Till)
	} else {
		toAdd := set.NewSet()
		toRemove := set.NewSet()
		// Segment exists, must add new members and remove old ones
		for _, key := range segmentChanges.Added {
			toAdd.Add(key)
		}
		for _, key := range segmentChanges.Removed {
			toRemove.Add(key)
		}
		if toAdd.Size() > 0 || toRemove.Size() > 0 {
			s.logger.Debug(fmt.Sprintf("Segment [%s] exists, it will be updated. %d keys added, %d keys removed", name, toAdd.Size(), toRemove.Size()))
			s.segmentStorage.Update(name, toAdd, toRemove, segmentChanges.Till)
		}
	}
}

// SynchronizeSegment syncs segment
func (s *SegmentFetcherProxy) SynchronizeSegment(name string, till *int64, requestNoCache bool) error {
	for {
		s.logger.Debug(fmt.Sprintf("Synchronizing segment %s", name))
		changeNumber, _ := s.segmentStorage.ChangeNumber(name)
		if changeNumber == 0 {
			changeNumber = -1
		}
		if till != nil && *till < changeNumber {
			return nil
		}

		before := time.Now()
		segmentChanges, err := s.segmentFetcher.Fetch(name, changeNumber, requestNoCache)
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				s.metricsWrapper.StoreCounters(storage.SegmentChangesCounter, strconv.Itoa(httpError.Code))
			}
			return err
		}

		for _, removedKey := range segmentChanges.Removed {
			s.logger.Debug("Removing", segmentChanges.Name, "for", removedKey)
			s.mySegments.RemoveSegmentForUser(removedKey, segmentChanges.Name)
		}

		for _, addedKey := range segmentChanges.Added {
			s.logger.Debug("Adding", segmentChanges.Name, "for", addedKey)
			s.mySegments.AddSegmentToUser(addedKey, segmentChanges.Name)
		}

		s.processUpdate(segmentChanges)

		bucket := util.Bucket(time.Since(before).Nanoseconds())
		s.metricsWrapper.StoreLatencies(storage.SegmentChangesLatency, bucket)
		s.metricsWrapper.StoreCounters(storage.SegmentChangesCounter, "ok")
		if segmentChanges.Till == segmentChanges.Since || (till != nil && segmentChanges.Till >= *till) {
			return nil
		}
	}
}

// IsSegmentCached returns if the segment exists instorage
func (s *SegmentFetcherProxy) IsSegmentCached(name string) bool {
	cn, err := s.segmentStorage.ChangeNumber(name)
	if err != nil {
		return false
	}
	return cn != -1
}
