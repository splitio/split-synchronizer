package worker

import (
	"errors"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/common"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/task"
)

// RecorderEventMultiple struct for event sync
type RecorderEventMultiple struct {
	eventStorage   storage.EventStorageConsumer
	eventRecorder  service.EventsRecorder
	metricsWrapper *storage.MetricWrapper
	mutext         *sync.Mutex
	logger         logging.LoggerInterface
}

// NewEventRecorderMultiple creates new event synchronizer for posting events
func NewEventRecorderMultiple(
	eventStorage storage.EventStorageConsumer,
	eventRecorder service.EventsRecorder,
	metricsWrapper *storage.MetricWrapper,
	logger logging.LoggerInterface,
) event.EventRecorder {
	return &RecorderEventMultiple{
		eventStorage:   eventStorage,
		eventRecorder:  eventRecorder,
		metricsWrapper: metricsWrapper,
		mutext:         &sync.Mutex{},
		logger:         logger,
	}
}

func (e *RecorderEventMultiple) fetchEvents(bulkSize int64) (map[dtos.Metadata][]dtos.EventDTO, error) {
	e.mutext.Lock()
	defer e.mutext.Unlock()

	storedEvents, err := e.eventStorage.PopNWithMetadata(bulkSize) //PopN has a mutex, so this function can be async without issues
	if err != nil {
		e.logger.Error("(Task) Post Events fails fetching events from storage", err.Error())
		return nil, err
	}
	// grouping the information by instanceID/instanceIP
	collectedData := make(map[dtos.Metadata][]dtos.EventDTO)

	for _, stored := range storedEvents {
		_, instanceExists := collectedData[stored.Metadata]
		if !instanceExists {
			collectedData[stored.Metadata] = make([]dtos.EventDTO, 0)
		}

		collectedData[stored.Metadata] = append(
			collectedData[stored.Metadata],
			stored.Event,
		)
	}

	return collectedData, nil
}

func (e *RecorderEventMultiple) synchronizeEvents(bulkSize int64) error {
	eventsToSend, err := e.fetchEvents(bulkSize)
	if err != nil {
		return err
	}

	// e.logger.Info(fmt.Sprintf("eventsToSend: %v", eventsToSend))
	for metadata, events := range eventsToSend {
		before := time.Now()
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			task.StoreDataFlushed(before.UnixNano(), len(events), e.eventStorage.Count(), "events")
		}
		err := common.WithAttempts(3, func() error {
			e.logger.Info("eventToSend: ", len(events))
			err := e.eventRecorder.Record(events, metadata)
			if err != nil {
				e.logger.Error("Error posting events")
			}

			return nil
		})
		if err != nil {
			if _, ok := err.(*dtos.HTTPError); ok {
				e.metricsWrapper.StoreCounters(storage.PostEventsCounter, string(err.(*dtos.HTTPError).Code), false)
			}
			return err
		}
		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		e.metricsWrapper.StoreLatencies(storage.PostEventsLatency, bucket, false)
		e.metricsWrapper.StoreCounters(storage.PostEventsCounter, "ok", false)
	}
	return nil
}

// SynchronizeEvents syncs events
func (e *RecorderEventMultiple) SynchronizeEvents(bulkSize int64) error {
	if task.IsOperationRunning(task.EventsOperation) {
		e.logger.Debug("Another task executed by the user is performing operations on Events. Skipping.")
		return nil
	}

	return e.synchronizeEvents(bulkSize)
}

// FlushEvents flushes events
func (e *RecorderEventMultiple) FlushEvents(bulkSize int64) error {
	if task.RequestOperation(task.EventsOperation) {
		defer task.FinishOperation(task.EventsOperation)
	} else {
		e.logger.Debug("Cannot execute flush. Another operation is performing operations on Events.")
		return errors.New("Cannot execute flush. Another operation is performing operations on Events")
	}
	elementsToFlush := splitio.MaxSizeToFlush
	if bulkSize != 0 {
		elementsToFlush = bulkSize
	}

	for elementsToFlush > 0 && e.eventStorage.Count() > 0 {
		maxSize := splitio.DefaultSize
		if elementsToFlush < splitio.DefaultSize {
			maxSize = elementsToFlush
		}
		err := e.synchronizeEvents(maxSize)
		if err != nil {
			return err
		}
		elementsToFlush = elementsToFlush - splitio.DefaultSize
	}
	return nil
}
