package task

import (
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/worker"
)

func NewImpressionCountSyncTask(
	wrk worker.ImpressionsCounstWorkerImp,
	logger logging.LoggerInterface,
	period int,
) *asynctask.AsyncTask {
	doWork := func(l logging.LoggerInterface) error {
		wrk.Process()
		return nil
	}

	return asynctask.NewAsyncTask("sync-impression-counts", doWork, period, nil, nil, logger)
}
