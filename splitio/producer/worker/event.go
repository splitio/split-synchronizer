package worker

import (
	"strings"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/common"
	"github.com/splitio/go-toolkit/logging"
)

const (
	postEventsLatencies = "events.time"
	postEventsCounters  = "events.status.{status}"
)

// RecorderEventMultiple struct for event sync
type RecorderEventMultiple struct {
	eventStorage  storage.EventStorageConsumer
	eventRecorder service.EventsRecorder
	metricStorage storage.MetricsStorageProducer
	mutext        *sync.Mutex
	logger        logging.LoggerInterface
}

// NewEventRecorderMultiple creates new event synchronizer for posting events
func NewEventRecorderMultiple(
	eventStorage storage.EventStorageConsumer,
	eventRecorder service.EventsRecorder,
	metricStorage storage.MetricsStorageProducer,
	logger logging.LoggerInterface,
) event.EventRecorder {
	return &RecorderEventMultiple{
		eventStorage:  eventStorage,
		eventRecorder: eventRecorder,
		metricStorage: metricStorage,
		mutext:        &sync.Mutex{},
		logger:        logger,
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

// SynchronizeEvents syncs events
func (e *RecorderEventMultiple) SynchronizeEvents(bulkSize int64) error {
	eventsToSend, err := e.fetchEvents(bulkSize)
	if err != nil {
		return err
	}

	for metadata, events := range eventsToSend {
		before := time.Now()
		err := common.WithAttempts(3, func() error {
			err := e.eventRecorder.Record(events, metadata)
			if err != nil {
				e.logger.Error("Error posting events")
			}

			return nil
		})
		if err != nil {
			if _, ok := err.(*dtos.HTTPError); ok {
				e.metricStorage.IncCounter(strings.Replace(postEventsCounters, "{status}", string(err.(*dtos.HTTPError).Code), 1))
			}
			return err
		}
		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		e.metricStorage.IncLatency(postEventsLatencies, bucket)
		e.metricStorage.IncCounter(strings.Replace(postEventsCounters, "{status}", "200", 1))
	}
	return nil
}

// FlushEvents flushes events
func (e *RecorderEventMultiple) FlushEvents(bulkSize int64) error {
	return nil
}
