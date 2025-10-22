package tasks

import (
	"fmt"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"

	"github.com/splitio/go-split-commons/v8/service/api"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/workerpool"
)

// EventWorker defines a component capable of recording imrpessions in raw form
type EventWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPEventsRecorder
}

// Name returns the name of the worker
func (w *EventWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *EventWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *EventWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *EventWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *EventWorker) DoWork(message interface{}) error {
	asEvents, ok := message.(*internal.RawEvents)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawEvents. Got '%T'", message))
		return nil
	}

	w.recorder.RecordRaw("/events/bulk", asEvents.Payload, asEvents.Metadata, nil)
	return nil
}

func newEventWorkerFactory(name string, recorder *api.HTTPEventsRecorder, logger logging.LoggerInterface) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &EventWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewEventsFlushTask creates a new impressions flushing task
func NewEventsFlushTask(recorder *api.HTTPEventsRecorder, logger logging.LoggerInterface, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(logger, newEventWorkerFactory("events-worker", recorder, logger), period, queueSize, threads)
}
