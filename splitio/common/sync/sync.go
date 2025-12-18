package sync

import (
	"github.com/splitio/go-split-commons/v9/conf"
	"github.com/splitio/go-split-commons/v9/synchronizer"
	"github.com/splitio/go-split-commons/v9/tasks"
	"github.com/splitio/go-toolkit/v5/logging"
)

// WSync is a wrapper for the Regular synchronizer that handles both local telemetry
// and user submitted telemetry
type WSync struct {
	synchronizer.Synchronizer
	logger             logging.LoggerInterface
	userTelemetryTasks []tasks.Task
}

// NewSynchronizer instantiates a producer-mode ready syncrhonizer that handles sdk-telemetry
func NewSynchronizer(
	confAdvanced conf.AdvancedConfig,
	splitTasks synchronizer.SplitTasks,
	workers synchronizer.Workers,
	logger logging.LoggerInterface,
	inMememoryFullQueue chan string,
	userTelemetryTasks []tasks.Task,
) *WSync {
	return &WSync{
		Synchronizer:       synchronizer.NewSynchronizer(confAdvanced, splitTasks, workers, logger, inMememoryFullQueue),
		logger:             logger,
		userTelemetryTasks: userTelemetryTasks,
	}
}

// StartPeriodicDataRecording starts periodic recorders tasks
func (s *WSync) StartPeriodicDataRecording() {
	s.Synchronizer.StartPeriodicDataRecording()
	for _, t := range s.userTelemetryTasks {
		t.Start()
	}
}

// StopPeriodicDataRecording stops periodic recorders tasks
func (s *WSync) StopPeriodicDataRecording() {
	s.Synchronizer.StopPeriodicDataRecording()
	for _, t := range s.userTelemetryTasks {
		t.Stop(true)
	}
}

// assert interface compliance
var _ synchronizer.Synchronizer = (*WSync)(nil)
