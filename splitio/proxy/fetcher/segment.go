package fetcher

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/service"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-split-commons/v2/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v2/util"
	"github.com/splitio/go-toolkit/v3/datastructures/set"
	"github.com/splitio/go-toolkit/v3/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
)

// SegmentFetcherProxy struct
type SegmentFetcherProxy struct {
	segmentStorage collections.SegmentChangesCollection
	splitStorage   collections.SplitChangesCollection
	segmentFetcher service.SegmentFetcher
	metricsWrapper *storage.MetricWrapper
	logger         logging.LoggerInterface
}

// NewSegmentFetcher build new fetcher for proxy
func NewSegmentFetcher(segmentStorage collections.SegmentChangesCollection, splitStorage collections.SplitChangesCollection, segmentFetcher service.SegmentFetcher, metricsWrapper *storage.MetricWrapper, logger logging.LoggerInterface) segment.SegmentFetcher {
	return &SegmentFetcherProxy{
		segmentStorage: segmentStorage,
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
func (s *SegmentFetcherProxy) SynchronizeSegments() error {
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
				err = s.SynchronizeSegment(segmentName, nil)
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

// SynchronizeSegment syncs segment
func (s *SegmentFetcherProxy) SynchronizeSegment(name string, till *int64) error {
	for {
		s.logger.Debug(fmt.Sprintf("Synchronizing segment %s", name))
		changeNumber := s.segmentStorage.ChangeNumber(name)
		if changeNumber == 0 {
			changeNumber = -1
		}
		if till != nil && *till < changeNumber {
			return nil
		}

		before := time.Now()
		segmentChanges, err := s.segmentFetcher.Fetch(name, changeNumber)
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				s.metricsWrapper.StoreCounters(storage.SegmentChangesCounter, string(httpError.Code))
			}
			return err
		}

		segmentItem, _ := s.segmentStorage.Fetch(segmentChanges.Name)

		if segmentItem == nil {
			segmentItem = &collections.SegmentChangesItem{}
			segmentItem.Name = segmentChanges.Name
			segmentItem.Keys = make(map[string]collections.SegmentKey)
		}

		for _, removedSegment := range segmentChanges.Removed {
			s.logger.Debug("Removing", removedSegment, "from", segmentChanges.Name)
			if _, exists := segmentItem.Keys[removedSegment]; exists {
				itemAux := segmentItem.Keys[removedSegment]
				itemAux.Removed = true
				itemAux.ChangeNumber = segmentChanges.Till
				segmentItem.Keys[removedSegment] = itemAux
			} else {
				segmentItem.Keys[removedSegment] = collections.SegmentKey{Name: removedSegment,
					Removed: true, ChangeNumber: segmentChanges.Till}
			}

		}

		for _, addedSegment := range segmentChanges.Added {
			s.logger.Debug("Adding", addedSegment, "in", segmentChanges.Name)
			if _, exists := segmentItem.Keys[addedSegment]; exists {
				itemAux := segmentItem.Keys[addedSegment]
				itemAux.Removed = false
				itemAux.ChangeNumber = segmentChanges.Till
				segmentItem.Keys[addedSegment] = itemAux
			} else {
				segmentItem.Keys[addedSegment] = collections.SegmentKey{Name: addedSegment,
					Removed: false, ChangeNumber: segmentChanges.Till}
			}
		}
		err = s.segmentStorage.Add(segmentItem)
		if err != nil {
			s.logger.Error(err)
			return err
		}
		s.segmentStorage.SetChangeNumber(segmentChanges.Name, segmentChanges.Till)

		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		s.metricsWrapper.StoreLatencies(storage.SegmentChangesLatency, bucket)
		s.metricsWrapper.StoreCounters(storage.SegmentChangesCounter, "ok")
		if segmentChanges.Till == segmentChanges.Since || (till != nil && segmentChanges.Till >= *till) {
			return nil
		}
	}
}
