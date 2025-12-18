package tasks

import (
	"fmt"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"

	"github.com/splitio/go-split-commons/v9/service/api"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/workerpool"
)

// CONFIG

// TelemetryConfigWorker defines a component capable of recording imrpessions in raw form
type TelemetryConfigWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPTelemetryRecorder
}

// Name returns the name of the worker
func (w *TelemetryConfigWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *TelemetryConfigWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *TelemetryConfigWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *TelemetryConfigWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *TelemetryConfigWorker) DoWork(message interface{}) error {
	asTelemetryConfig, ok := message.(*internal.RawTelemetryConfig)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawTelemetryConfig. Got '%T'", message))
		return nil
	}

	w.recorder.RecordRaw("/metrics/config", asTelemetryConfig.Payload, asTelemetryConfig.Metadata, nil)
	return nil
}

func newTelemetryConfigWorkerFactory(name string, recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &TelemetryConfigWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewTelemetryConfigFlushTask creates a new impressions flushing task
func NewTelemetryConfigFlushTask(recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(logger, newTelemetryConfigWorkerFactory("telemetry-config-worker", recorder, logger), period, queueSize, threads)
}

// USAGE

// TelemetryUsageWorker defines a component capable of recording imrpessions in raw form
type TelemetryUsageWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPTelemetryRecorder
}

// Name returns the name of the worker
func (w *TelemetryUsageWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *TelemetryUsageWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *TelemetryUsageWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *TelemetryUsageWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *TelemetryUsageWorker) DoWork(message interface{}) error {
	asTelemetryUsage, ok := message.(*internal.RawTelemetryUsage)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawTelemetryUsage. Got '%T'", message))
		return nil
	}

	w.recorder.RecordRaw("/metrics/usage", asTelemetryUsage.Payload, asTelemetryUsage.Metadata, nil)
	return nil
}

func newTelemetryUsageWorkerFactory(name string, recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &TelemetryUsageWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewTelemetryUsageFlushTask creates a new impressions flushing task
func NewTelemetryUsageFlushTask(recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(logger, newTelemetryUsageWorkerFactory("telemetry-config-worker", recorder, logger), period, queueSize, threads)
}

// Keys Client Side

// TelemetryKeysClientSideWorker defines a component capable of recording mtk client side in raw form
type TelemetryKeysClientSideWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPTelemetryRecorder
}

// Name returns the name of the worker
func (w *TelemetryKeysClientSideWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *TelemetryKeysClientSideWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *TelemetryKeysClientSideWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *TelemetryKeysClientSideWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *TelemetryKeysClientSideWorker) DoWork(message interface{}) error {
	asTelemetryKeysClientSide, ok := message.(*internal.RawKeysClientSide)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawKeysClientSide. Got '%T'", message))
		return nil
	}

	w.recorder.RecordRaw("/keys/cs", asTelemetryKeysClientSide.Payload, asTelemetryKeysClientSide.Metadata, nil)
	return nil
}

func newTelemetryKeysClientSideWorkerFactory(name string, recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &TelemetryKeysClientSideWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewTelemetryKeysClientSideFlushTask creates a new flushing task
func NewTelemetryKeysClientSideFlushTask(recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(logger, newTelemetryKeysClientSideWorkerFactory("telemetry-keys-client-side-worker", recorder, logger), period, queueSize, threads)
}

// Keys Server Side

// TelemetryKeysServerSideWorker defines a component capable of recording mtk server side in raw form
type TelemetryKeysServerSideWorker struct {
	name     string
	logger   logging.LoggerInterface
	recorder *api.HTTPTelemetryRecorder
}

// Name returns the name of the worker
func (w *TelemetryKeysServerSideWorker) Name() string { return w.name }

// OnError is called whenever theres an error in the worker function
func (w *TelemetryKeysServerSideWorker) OnError(e error) {}

// Cleanup is called after the worker is shutdown
func (w *TelemetryKeysServerSideWorker) Cleanup() error { return nil }

// FailureTime specifies how long to wait when an errors occurs before executing again
func (w *TelemetryKeysServerSideWorker) FailureTime() int64 { return 1 }

// DoWork is called and passed a message fetched from the work queue
func (w *TelemetryKeysServerSideWorker) DoWork(message interface{}) error {
	asTelemetryKeysServerSide, ok := message.(*internal.RawKeysServerSide)
	if !ok {
		w.logger.Error(fmt.Sprintf("invalid data fetched from queue. Expected RawKeysServerSide. Got '%T'", message))
		return nil
	}

	w.recorder.RecordRaw("/keys/ss", asTelemetryKeysServerSide.Payload, asTelemetryKeysServerSide.Metadata, nil)
	return nil
}

func newTelemetryKeysServerSideWorkerWorkerFactory(name string, recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface) WorkerFactory {
	var i *int = common.IntRef(0)
	return func() workerpool.Worker {
		defer func() { *i++ }()
		return &TelemetryKeysServerSideWorker{name: fmt.Sprintf("%s_%d", name, i), logger: logger, recorder: recorder}
	}
}

// NewTelemetryKeysServerSideFlushTask creates a new flushing task
func NewTelemetryKeysServerSideFlushTask(recorder *api.HTTPTelemetryRecorder, logger logging.LoggerInterface, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	return newDeferredFlushTask(logger, newTelemetryKeysServerSideWorkerWorkerFactory("telemetry-keys-server-side-worker", recorder, logger), period, queueSize, threads)
}
