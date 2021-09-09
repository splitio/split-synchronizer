package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/provisional"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/telemetry"
	commonToolkit "github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
)

const (
	impressionObserverCacheSize = 500000
)

// ErrImpressionsSyncFailed is returned when events synchronization fails
var ErrImpressionsSyncFailed = errors.New("impressions synchronization failed for at least one sdk instance")

// RecorderImpressionMultiple struct for impression sync
type RecorderImpressionMultiple struct {
	impressionStorage  storage.ImpressionMultiSdkConsumer
	impressionRecorder service.ImpressionsRecorder
	localTelemetry     storage.TelemetryRuntimeProducer
	listener           impressionlistener.ImpressionBulkListener
	logger             logging.LoggerInterface
	impressionManager  provisional.ImpressionManager
	mode               string
	evictionMonitor    evcalc.Monitor
}

// NewImpressionRecordMultiple creates new impression synchronizer for posting impressions
func NewImpressionRecordMultiple(
	impressionStorage storage.ImpressionMultiSdkConsumer,
	impressionRecorder service.ImpressionsRecorder,
	listener impressionlistener.ImpressionBulkListener,
	localTelemetry storage.TelemetryRuntimeProducer,
	logger logging.LoggerInterface,
	managerConfig conf.ManagerConfig,
	impressionsCounter *provisional.ImpressionsCounter,
	evictionMonitor evcalc.Monitor,
) (*RecorderImpressionMultiple, error) {
	impressionManager, err := provisional.NewImpressionManager(managerConfig, impressionsCounter, localTelemetry)
	if err != nil {
		return nil, err
	}
	return &RecorderImpressionMultiple{
		impressionStorage:  impressionStorage,
		impressionRecorder: impressionRecorder,
		listener:           listener,
		localTelemetry:     localTelemetry,
		logger:             logger,
		impressionManager:  impressionManager,
		mode:               managerConfig.ImpressionsMode,
		evictionMonitor:    evictionMonitor,
	}, nil
}

// SynchronizeImpressions syncs impressions
func (r *RecorderImpressionMultiple) SynchronizeImpressions(bulkSize int64) error {
	if r.evictionMonitor.Busy() {
		r.logger.Debug("Another task executed by the user is performing operations on Impressions. Skipping.")
		return nil
	}

	return r.synchronizeImpressions(bulkSize)
}

// FlushImpressions flushes impressions
func (r *RecorderImpressionMultiple) FlushImpressions(bulkSize int64) error {
	if r.evictionMonitor.Acquire() {
		defer r.evictionMonitor.Release()
	} else {
		r.logger.Debug("Cannot execute flush. Another operation is performing operations on Impressions.")
		return errors.New("Cannot execute flush. Another operation is performing operations on Impressions")
	}
	elementsToFlush := maxFlushSize

	if bulkSize != 0 {
		elementsToFlush = bulkSize
	}

	for elementsToFlush > 0 && r.impressionStorage.Count() > 0 {
		maxSize := defaultFlushSize
		if elementsToFlush < defaultFlushSize {
			maxSize = elementsToFlush
		}
		err := r.synchronizeImpressions(maxSize)
		if err != nil {
			return err
		}
		elementsToFlush = elementsToFlush - defaultFlushSize
	}
	return nil
}

func (r *RecorderImpressionMultiple) synchronizeImpressions(bulkSize int64) error {
	impressionsToSend, impressionsForListener, totalCount, err := r.fetch(bulkSize)
	if err != nil {
		return err
	}

	err = r.recordImpressions(impressionsToSend, totalCount)
	if err != nil {
		return err
	}

	r.sendDataToListener(impressionsForListener)
	return nil
}

func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (impressionsByMetadata, listenerImpressionsByMetadata, int, error) {
	storedImpressions, err := r.impressionStorage.PopNWithMetadata(bulkSize) // PopN has a mutex, so this function can be async without issues
	if err != nil {
		r.logger.Error("(Task) Post Impressions fails fetching impressions from storage", err.Error())
		return nil, nil, 0, err
	}

	// grouping the information by instanceID/instanceIP, and then by feature name
	collectedDataforLog := make(impressionsByMetadataByFeature)
	collectedDataforListener := make(listenerImpressionsByMetadataByFeature)

	for _, stored := range storedImpressions {
		toSend, forListener := r.impressionManager.ProcessImpressions([]dtos.Impression{stored.Impression})
		collectedDataforLog = wrapData(toSend, collectedDataforLog, stored.Metadata)
		collectedDataforListener = wrapDataForListener(forListener, collectedDataforListener, stored.Metadata)
	}
	return r.wrapDTO(collectedDataforLog), wrapDTOListener(collectedDataforListener), len(storedImpressions), nil
}

func (r *RecorderImpressionMultiple) recordImpressions(impressionsToSend impressionsByMetadata, totalFetched int) error {
	errs := 0
	for metadata, impressions := range impressionsToSend {
		before := time.Now()
		r.evictionMonitor.StoreDataFlushed(before, totalFetched, r.impressionStorage.Count())
		err := commonToolkit.WithAttempts(3, func() error {
			r.logger.Debug("impressionsToSend: ", len(impressions))
			err := r.impressionRecorder.Record(impressions, metadata, map[string]string{"SplitSDKImpressionsMode": r.mode})
			if err != nil {
				if httpError, ok := err.(*dtos.HTTPError); ok {
					r.localTelemetry.RecordSyncError(telemetry.ImpressionSync, httpError.Code)
				}
			}
			return err
		})
		if err != nil {
			errs++
			r.logger.Error(fmt.Sprintf("Error posting impressions for metadata '%+v' after 3 attempts. Data will be discarded", metadata))
		}

		r.localTelemetry.RecordSyncLatency(telemetry.ImpressionSync, time.Now().Sub(before))
		r.localTelemetry.RecordSuccessfulSync(telemetry.ImpressionSync, time.Now().UTC())
	}

	if errs > 0 {
		return ErrImpressionsSyncFailed
	}

	return nil
}

func (r *RecorderImpressionMultiple) sendDataToListener(impressionsToListener listenerImpressionsByMetadata) {
	if r.listener == nil {
		return
	}
	for metadata, impressions := range impressionsToListener {
		rawImpressions, err := json.Marshal(impressions)
		if err != nil {
			r.logger.Error("JSON encoding failed for the following impressions", impressions, metadata)
			continue
		}

		err = r.listener.Submit(rawImpressions, &metadata)
		if err != nil {
			r.logger.Error("error queuing impressions for listener: ", err)
		}
	}
}

func (r *RecorderImpressionMultiple) wrapDTO(collectedData impressionsByMetadataByFeature) impressionsByMetadata {
	var err error
	impressions := make(impressionsByMetadata)
	for metadata, impsForMetadata := range collectedData {
		impressions[metadata], err = toImpressionsDTO(impsForMetadata)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Unable to write impressions for metadata %v", metadata))
			continue
		}
	}
	return impressions
}

// auxiliary type aliases
type impressionsByMetadata = map[dtos.Metadata][]dtos.ImpressionsDTO
type impressionsByMetadataByFeature = map[dtos.Metadata]map[string][]dtos.ImpressionDTO
type impressionsByFeature = map[string][]dtos.ImpressionDTO
type listenerImpressionsByMetadata = map[dtos.Metadata][]common.ImpressionsForListener
type listenerImpressionsByMetadataByFeature = map[dtos.Metadata]map[string][]common.ImpressionForListener
type listenerImpressionsByFeature = map[string][]common.ImpressionForListener
