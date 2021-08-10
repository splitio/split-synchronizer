package sync

import (
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/worker"
)

// WSync is a wrapper for the Regular synchronizer that handles local telemetry (regularly)
// and adds an extra task for SDK generated telemetry
type WSync struct {
	synchronizer.Synchronizer
	logger           logging.LoggerInterface
	sdkTelemetryTask *asynctask.AsyncTask
}

// NewSynchronizer instantiates a producer-mode ready syncrhonizer that handles sdk-telemetry
func NewSynchronizer(
	confAdvanced conf.AdvancedConfig,
	splitTasks synchronizer.SplitTasks,
	workers synchronizer.Workers,
	logger logging.LoggerInterface,
	inMememoryFullQueue chan string,
	sdkTelemetryWorker worker.TelemetryMultiWorker,
	periodSecs int,
) *WSync {
	return &WSync{
		Synchronizer:     synchronizer.NewSynchronizer(confAdvanced, splitTasks, workers, logger, inMememoryFullQueue),
		logger:           logger,
		sdkTelemetryTask: makeSDKTelemetryTask(sdkTelemetryWorker, logger, periodSecs),
	}
}

// StartPeriodicDataRecording starts periodic recorders tasks
func (s *WSync) StartPeriodicDataRecording() {
	s.Synchronizer.StartPeriodicDataRecording()
	if s.sdkTelemetryTask != nil {
		s.sdkTelemetryTask.Start()
	}
}

// StopPeriodicDataRecording stops periodic recorders tasks
func (s *WSync) StopPeriodicDataRecording() {
	s.Synchronizer.StopPeriodicDataRecording()
	if s.sdkTelemetryTask != nil {
		s.sdkTelemetryTask.Stop(true)
	}
}

func makeSDKTelemetryTask(tWorker worker.TelemetryMultiWorker, logger logging.LoggerInterface, periodSecs int) *asynctask.AsyncTask {
	return asynctask.NewAsyncTask(
		"sdk-telemetry-recorder",
		func(l logging.LoggerInterface) error {
			err := tWorker.SyncrhonizeConfigs()
			if err != nil {
				l.Error("error submiting sdk telemetry::stats: ", err.Error())
			}
			err = tWorker.SynchronizeStats()
			if err != nil {
				l.Error("error submiting sdk telemetry::config: ", err.Error())
			}
			return nil
		},
		periodSecs,
		nil, // no init required
		nil, // no flushing on stop
		logger,
	)
}

// assert interface compliance
var _ synchronizer.Synchronizer = (*WSync)(nil)
