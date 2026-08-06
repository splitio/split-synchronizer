package task

import (
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/worker"
)

// NewTelemetrySyncTask constructs a task used to periodically record sdk configs and stats into the Split servers
func NewTelemetrySyncTask(wrk worker.TelemetryMultiWorker, logger logging.LoggerInterface, period int) *asynctask.AsyncTask {
	doWork := func(l logging.LoggerInterface) error {
		wrk.SynchronizeStats()
		wrk.SyncrhonizeConfigs()
		return nil
	}
	return asynctask.NewAsyncTask("sdk-telemetry", doWork, period, nil, nil, logger)
}
