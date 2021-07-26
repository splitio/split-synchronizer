package fetcher

import (
	"strconv"
	"time"

	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-split-commons/v3/service"
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/split"
	"github.com/splitio/go-split-commons/v3/util"
	"github.com/splitio/go-toolkit/v4/logging"

	//	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	storageV2 "github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/v2"
)

// SplitFetcherProxy struct
type SplitFetcherProxy struct {
	splitStorage          storage.SplitStorage
	splitChangesSummaries *storageV2.SplitChangesSummaries
	splitFetcher          service.SplitFetcher
	metricsWrapper        *storage.MetricWrapper
	httpCache             *gincache.Middleware
	logger                logging.LoggerInterface
}

// NewSplitFetcher build new fetcher for proxy
func NewSplitFetcher(
	splitStorage storage.SplitStorage,
	splitChangesSummaries *storageV2.SplitChangesSummaries,
	splitFetcher service.SplitFetcher,
	metricsWrapper *storage.MetricWrapper,
	httpCache *gincache.Middleware,
	logger logging.LoggerInterface,
) split.Updater {
	return &SplitFetcherProxy{
		splitChangesSummaries: splitChangesSummaries,
		splitStorage:          splitStorage,
		splitFetcher:          splitFetcher,
		metricsWrapper:        metricsWrapper,
		logger:                logger,
		httpCache:             httpCache,
	}
}

// SynchronizeSplits syncs splits
func (s *SplitFetcherProxy) SynchronizeSplits(till *int64, requestNoCache bool) ([]string, error) {
	// @TODO: add delays
	for {
		changeNumber, _ := s.splitStorage.ChangeNumber()
		if changeNumber == 0 {
			changeNumber = -1
		}
		if till != nil && *till < changeNumber {
			return nil, nil
		}

		before := time.Now()
		splits, err := s.splitFetcher.Fetch(changeNumber, requestNoCache)
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				s.metricsWrapper.StoreCounters(storage.SplitChangesCounter, strconv.Itoa(httpError.Code))
			}
			return nil, err
		}

		toAdd := []dtos.SplitDTO{}
		toDel := []dtos.SplitDTO{}
		toAddView := []storageV2.SplitMinimalView{}
		toDelView := []storageV2.SplitMinimalView{}
		for _, split := range splits.Splits {
			if split.Status == "ACTIVE" {
				toAdd = append(toAdd, split)
				toAddView = append(toAddView, storageV2.SplitMinimalView{Name: split.Name, TrafficType: split.TrafficTypeName})
			} else {
				toDel = append(toDel, split)
				toDelView = append(toDelView, storageV2.SplitMinimalView{Name: split.Name, TrafficType: split.TrafficTypeName})
			}
		}
		s.splitStorage.PutMany(toAdd, splits.Till)
		for _, spl := range toDel {
			s.splitStorage.Remove(spl.Name)
		}
		s.splitChangesSummaries.AddChanges(splits.Till, toAddView, toDelView)
		if len(toAdd) > 0 || len(toDel) > 0 {
			s.httpCache.EvictAll()
		}

		bucket := util.Bucket(time.Since(before).Nanoseconds())
		s.metricsWrapper.StoreCounters(storage.SplitChangesCounter, "ok")
		s.metricsWrapper.StoreLatencies(storage.SplitChangesLatency, bucket)
		if splits.Till == splits.Since || (till != nil && splits.Till >= *till) {
			return nil, nil
		}
	}
}

// LocalKill does nothing in proxy mode
func (s *SplitFetcherProxy) LocalKill(string, string, int64) { /* no-op */ }
