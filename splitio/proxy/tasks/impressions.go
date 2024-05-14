package tasks

import (
	"fmt"

	"github.com/splitio/go-split-commons/v6/service/api"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/workerpool"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
)

// ImpressionWorker defines a component capable of recording imrpessions in raw form
type ImpressionWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPImpressionRecorder
}

// Name returns the name of the worker
func (w *ImpressionWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *ImpressionWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *ImpressionWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *ImpressionWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *ImpressionWorker) DoWork(message interface{}) error {
	asImpressions, ok := message.(*internal.RawImpressions)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawImpressions. Got '%T'", message))
		return nil
	}

	extraHeaders := map[string]string{"SDKImpressionsMode": asImpressions.Mode}
	err := w.recorder.RecordRaw("/testImpressions/bulk", asImpressions.Payload, asImpressions.Metadata, extraHeaders)

	if err != nil {
		return fmt.Errorf("error posting impressions to Split servers: %w", err)
	}
	return nil
}

func newImpressionWorkerFactory(
	name string,
	recorder *api.HTTPImpressionRecorder,
	logger logging.LoggerInterface,
) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &ImpressionWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewImpressionsFlushTask creates a new impressions flushing task
func NewImpressionsFlushTask(
	recorder *api.HTTPImpressionRecorder,
	logger logging.LoggerInterface,
	period int,
	queueSize int,
	threads int,
) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(
		logger,
		newImpressionWorkerFactory("impressions-worker", recorder, logger),
		period,
		queueSize,
		threads,
	)
}
