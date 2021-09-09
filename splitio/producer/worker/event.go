package worker

import (
	"errors"
	"fmt"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
)

// ErrEventsSyncFailed is returned when events synchronization fails
var ErrEventsSyncFailed = errors.New("event synchronization failed for at least one sdk instance")

// RecorderEventMultiple struct for event sync
type RecorderEventMultiple struct {
	eventStorage    storage.EventMultiSdkConsumer
	eventRecorder   service.EventsRecorder
	localTelemetry  storage.TelemetryRuntimeProducer
	evictionMonitor evcalc.Monitor
	logger          logging.LoggerInterface
}

// NewEventRecorderMultiple creates new event synchronizer for posting events
func NewEventRecorderMultiple(
	eventStorage storage.EventMultiSdkConsumer,
	eventRecorder service.EventsRecorder,
	localTelemetry storage.TelemetryRuntimeProducer,
	evictionMonitor evcalc.Monitor,
	logger logging.LoggerInterface,
) event.EventRecorder {
	return &RecorderEventMultiple{
		eventStorage:    eventStorage,
		eventRecorder:   eventRecorder,
		localTelemetry:  localTelemetry,
		evictionMonitor: evictionMonitor,
		logger:          logger,
	}
}

// SynchronizeEvents syncs events
func (e *RecorderEventMultiple) SynchronizeEvents(bulkSize int64) error {
	// We don't lock here since we might have multiple threads calling SynchronizeEvents, which is harmless.
	if e.evictionMonitor.Busy() {
		e.logger.Info("A user requested drop/flush is in progress. Skipping this iteration of periodic event flush")
		return nil
	}
	return e.synchronizeEvents(bulkSize)
}

// FlushEvents flushes events
func (e *RecorderEventMultiple) FlushEvents(bulkSize int64) error {
	if e.evictionMonitor.Acquire() {
		defer e.evictionMonitor.Release()
	} else {
		e.logger.Debug("Cannot execute flush. Another operation is performing operations on Events.")
		return errors.New("Cannot execute flush. Another operation is performing operations on Events")
	}
	elementsToFlush := maxFlushSize
	if bulkSize != 0 {
		elementsToFlush = bulkSize
	}

	for elementsToFlush > 0 && e.eventStorage.Count() > 0 {
		maxSize := defaultFlushSize
		if elementsToFlush < defaultFlushSize {
			maxSize = elementsToFlush
		}
		err := e.synchronizeEvents(maxSize)
		if err != nil {
			return err
		}
		elementsToFlush = elementsToFlush - defaultFlushSize
	}
	return nil
}

func (e *RecorderEventMultiple) synchronizeEvents(bulkSize int64) error {
	eventsToSend, err := e.fetchEvents(bulkSize)
	if err != nil {
		return fmt.Errorf("error fetching events from storage: %w", err)
	}

	errs := 0
	for metadata, events := range eventsToSend {
		before := time.Now()
		e.evictionMonitor.StoreDataFlushed(before, len(events), e.eventStorage.Count())
		err := common.WithAttempts(3, func() error {
			err := e.eventRecorder.Record(events, metadata)
			if err != nil {
				if httpError, ok := err.(*dtos.HTTPError); ok {
					e.localTelemetry.RecordSyncError(telemetry.EventSync, httpError.Code)
				}
			}
			return err
		})
		if err != nil {
			errs++
			e.logger.Error(fmt.Sprintf("Error posting events for metadata '%+v' after 3 attempts. Data will be discarded", metadata))
		}

		e.localTelemetry.RecordSyncLatency(telemetry.EventSync, time.Now().Sub(before))
		e.localTelemetry.RecordSuccessfulSync(telemetry.EventSync, time.Now().UTC())
	}

	if errs > 0 {
		return ErrEventsSyncFailed
	}
	return nil
}

func (e *RecorderEventMultiple) fetchEvents(bulkSize int64) (map[dtos.Metadata][]dtos.EventDTO, error) {
	storedEvents, err := e.eventStorage.PopNWithMetadata(bulkSize) //PopN has a mutex, so this function can be async without issues
	if err != nil {
		return nil, fmt.Errorf("error popping events w/metadata: %w", err)
	}
	// grouping the information by instanceID/instanceIP
	collectedData := make(map[dtos.Metadata][]dtos.EventDTO)

	for _, stored := range storedEvents {
		_, instanceExists := collectedData[stored.Metadata]
		if !instanceExists {
			collectedData[stored.Metadata] = make([]dtos.EventDTO, 0)
		}

		collectedData[stored.Metadata] = append(collectedData[stored.Metadata], stored.Event)
	}
	return collectedData, nil
}
