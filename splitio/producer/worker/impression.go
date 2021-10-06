package worker

import (
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
	impressionsToSend, impressionsForListener, err := r.fetch(bulkSize)
	if err != nil {
		return err
	}

	err = r.recordImpressions(impressionsToSend)
	if err != nil {
		return err
	}

	r.sendDataToListener(impressionsForListener)
	return nil
}

// func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (impressionsByMetadata, listenerImpressionsByMetadata, int, error) {
func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (beImpressionsByMetadataAndFeature, listenerImpressionsByMetadataAndFeature, error) {
	fetched, err := r.impressionStorage.PopNWithMetadata(bulkSize) // PopN has a mutex, so this function can be async without issues
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching impressions from storage: %w", err)
	}

	if len(fetched) == 0 { // Nothing in storage. Nothing to do here
		return nil, nil, nil
	}

	// Even though impressions are not yet sent, they've already been pulled from storage, so we might
	// as well update the eviction calculation for the lamabda now
	r.evictionMonitor.StoreDataFlushed(time.Now(), len(fetched), r.impressionStorage.Count())
	if err != nil {
		r.logger.Error("Error updating eviction calculation lambda for impressions: ", err.Error())
		return nil, nil, err
	}

	toListener := makeListenerPayloadBuilder()
	toBackend := makeBePayloadBuilder()
	for _, stored := range fetched {
		forBe, forListener := r.impressionManager.ProcessImpressions([]dtos.Impression{stored.Impression})
		if len(forBe) > 0 {
			toBackend.add(&forBe[0], &stored.Metadata)
		}
		if len(forListener) > 0 {
			toListener.add(&forListener[0], &stored.Metadata)
		}
	}
	return toBackend.accum, toListener.accum, err
}

func (r *RecorderImpressionMultiple) recordImpressions(impressionsToSend beImpressionsByMetadataAndFeature) error {
	errs := 0
	for metadata, byName := range impressionsToSend {
		asTestImpressions := toTestImpressionsSlice(byName)
		before := time.Now()
		err := commonToolkit.WithAttempts(3, func() error {
			r.logger.Debug("impressionsToSend: ", len(asTestImpressions))
			err := r.impressionRecorder.Record(asTestImpressions, metadata, map[string]string{"SplitSDKImpressionsMode": r.mode})
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

func (r *RecorderImpressionMultiple) sendDataToListener(impressionsToListener listenerImpressionsByMetadataAndFeature) {
	if r.listener == nil {
		return
	}
	for metadata, impressions := range impressionsToListener {
		byName := toListenerImpressionsSlice(impressions)
		err := r.listener.Submit(byName, &metadata)
		if err != nil {
			r.logger.Error("error queuing impressions for listener: ", err)
		}
	}
}
