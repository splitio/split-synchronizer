package fetcher

import (
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/split"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb/collections"
)

// SplitFetcherProxy struct
type SplitFetcherProxy struct {
	splitStorage   collections.SplitChangesCollection
	splitFetcher   service.SplitFetcher
	metricsWrapper *storage.MetricWrapper
	logger         logging.LoggerInterface
}

// NewSplitFetcher build new fetcher for proxy
func NewSplitFetcher(splitStorage collections.SplitChangesCollection, splitFetcher service.SplitFetcher, metricsWrapper *storage.MetricWrapper, logger logging.LoggerInterface) split.SplitFetcher {
	return &SplitFetcherProxy{
		splitStorage:   splitStorage,
		splitFetcher:   splitFetcher,
		metricsWrapper: metricsWrapper,
		logger:         logger,
	}
}

// SynchronizeSplits syncs splits
func (s *SplitFetcherProxy) SynchronizeSplits(till *int64) error {
	// @TODO: add delays
	for {
		changeNumber := s.splitStorage.ChangeNumber()
		if till != nil && *till < changeNumber {
			return nil
		}

		before := time.Now()
		splits, err := s.splitFetcher.Fetch(changeNumber)
		if err != nil {
			if _, ok := err.(*dtos.HTTPError); ok {
				s.metricsWrapper.StoreCounters(storage.SplitChangesCounter, string(err.(*dtos.HTTPError).Code))
			}
			return err
		}

		s.splitStorage.SetChangeNumber(splits.Till)
		for _, split := range splits.Splits {
			splitChangesItem := &collections.SplitChangesItem{}
			rdat, err := split.MarshalBinary()
			if err != nil {
				continue
			}
			splitChangesItem.JSON = string(rdat)
			splitChangesItem.ChangeNumber = split.ChangeNumber
			splitChangesItem.Name = split.Name
			splitChangesItem.Status = split.Status

			err = s.splitStorage.Add(splitChangesItem)
			if err != nil {
				continue
			}
		}

		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		s.metricsWrapper.StoreCounters(storage.SplitChangesCounter, "ok")
		s.metricsWrapper.StoreLatencies(storage.SplitChangesLatency, bucket)
		if splits.Till == splits.Since || (till != nil && splits.Till >= *till) {
			return nil
		}
	}
}
