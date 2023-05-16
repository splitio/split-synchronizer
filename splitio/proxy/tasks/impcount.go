package tasks

import (
	"fmt"

	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/workerpool"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
)

// ImpressionCountWorker defines a component capable of recording imrpessions in raw form
type ImpressionCountWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPImpressionRecorder
}

// Name returns the name of the worker
func (w *ImpressionCountWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *ImpressionCountWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *ImpressionCountWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *ImpressionCountWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *ImpressionCountWorker) DoWork(message interface{}) error {
	asCounts, ok := message.(*internal.RawImpressionCount)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawImpressions. Got '%T'", message))
		return nil
	}

	err := w.recorder.RecordRaw("/testImpressions/count", asCounts.Payload, asCounts.Metadata, nil)
	if err != nil {
		return fmt.Errorf("error posting impression counts to Split servers: %w", err)
	}
	return nil
}

func newImpressionCountWorkerFactory(
	name string,
	recorder *api.HTTPImpressionRecorder,
	logger logging.LoggerInterface,
) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &ImpressionCountWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewImpressionCountFlushTask creates a new impressions flushing task
func NewImpressionCountFlushTask(
	recorder *api.HTTPImpressionRecorder,
	logger logging.LoggerInterface,
	period int,
	queueSize int,
	threads int,
) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(
		logger,
		newImpressionCountWorkerFactory("impressions-count-worker", recorder, logger),
		period,
		queueSize,
		threads,
	)
}
