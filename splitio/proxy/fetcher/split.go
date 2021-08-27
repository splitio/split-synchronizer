package fetcher

import (
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/split"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
)

// SplitFetcherProxy struct
type SplitFetcherProxy struct {
	splitStorage   collections.SplitChangesCollection
	splitFetcher   service.SplitFetcher
	localTelemetry storage.TelemetryRuntimeProducer
	logger         logging.LoggerInterface
}

// NewSplitFetcher build new fetcher for proxy
func NewSplitFetcher(splitStorage collections.SplitChangesCollection, splitFetcher service.SplitFetcher, localTelemetry storage.TelemetryRuntimeProducer, logger logging.LoggerInterface) split.Updater {
	return &SplitFetcherProxy{
		splitStorage:   splitStorage,
		splitFetcher:   splitFetcher,
		localTelemetry: localTelemetry,
		logger:         logger,
	}
}

// SynchronizeSplits syncs splits
func (s *SplitFetcherProxy) SynchronizeSplits(till *int64, requestNoCache bool) ([]string, error) {
	// @TODO: add delays
	for {
		changeNumber := s.splitStorage.ChangeNumber()
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
				s.localTelemetry.RecordSyncError(telemetry.SplitSync, httpError.Code)
			}
			return nil, err
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

		s.localTelemetry.RecordSyncLatency(telemetry.SplitSync, time.Now().Sub(before))
		s.localTelemetry.RecordSuccessfulSync(telemetry.SplitSync, time.Now())
		if splits.Till == splits.Since || (till != nil && splits.Till >= *till) {
			return nil, nil
		}
	}
}

// LocalKill does nothing in proxy mode
func (s *SplitFetcherProxy) LocalKill(string, string, int64) { /* no-op */ }
