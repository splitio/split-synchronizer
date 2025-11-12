package worker

import (
	"fmt"

	"github.com/splitio/split-synchronizer/v5/splitio/producer/storage"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/service"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	tagConsumer = "consumer"
)

// TelemetryMultiSyncError is used to signal errors on multiple calls to telemetry recording apis
type TelemetryMultiSyncError struct {
	Errors map[dtos.Metadata]error
}

// Error returns the error formatted as a string
func (t *TelemetryMultiSyncError) Error() string {
	s := ""
	for metadata, err := range t.Errors {
		s += fmt.Sprintf("[%+v::%s],", metadata, err.Error())
	}
	return s
}

// TelemetryMultiWorker defines the interface for a telemetry syncrhonizer suitable for multiple sdk instances
type TelemetryMultiWorker interface {
	SynchronizeStats() error
	SyncrhonizeConfigs() error
}

// TelemetryMultiWorkerImpl is a component used to syncrhonize telemetry posted in redis by sdk
// into the split servers
type TelemetryMultiWorkerImpl struct {
	logger  logging.LoggerInterface
	storage storage.RedisTelemetryConsumerMulti
	sync    service.TelemetryRecorder
}

// NewTelemetryMultiWorker instantes a new telemetry worker
func NewTelemetryMultiWorker(logger logging.LoggerInterface, store storage.RedisTelemetryConsumerMulti, sync service.TelemetryRecorder) *TelemetryMultiWorkerImpl {
	return &TelemetryMultiWorkerImpl{
		logger:  logger,
		storage: store,
		sync:    sync,
	}
}

func (w *TelemetryMultiWorkerImpl) buildStats() map[dtos.Metadata]dtos.Stats {
	latencies := w.storage.PopLatencies()
	exceptions := w.storage.PopExceptions()

	toRet := make(map[dtos.Metadata]dtos.Stats)
	for metadata, lats := range latencies {
		latCopy := lats
		stats := newConsumerStats()
		stats.MethodLatencies = &latCopy
		if excs, ok := exceptions[metadata]; ok {
			stats.MethodExceptions = &excs
		}
		toRet[metadata] = stats
	}

	for metadata, excs := range exceptions {
		if current, ok := toRet[metadata]; !ok { // if the metadata exists, exceptions have already been stored
			excCopy := excs
			current = newConsumerStats()
			current.MethodExceptions = &excCopy
			toRet[metadata] = current
		}
	}

	return toRet
}

func newConsumerStats() dtos.Stats {
	return dtos.Stats{Tags: []string{tagConsumer}}
}

// SynchronizeStats syncs telemetry stats
func (w *TelemetryMultiWorkerImpl) SynchronizeStats() error {
	errors := make(map[dtos.Metadata]error)
	for metadata, stats := range w.buildStats() {
		err := w.sync.RecordStats(stats, metadata)
		if err != nil {
			errors[metadata] = err
		}
	}

	if len(errors) != 0 {
		return &TelemetryMultiSyncError{Errors: errors}
	}

	return nil
}

// SyncrhonizeConfigs syncs sdk configs
func (w *TelemetryMultiWorkerImpl) SyncrhonizeConfigs() error {
	errors := make(map[dtos.Metadata]error)
	for metadata, config := range w.storage.PopConfigs() {
		err := w.sync.RecordConfig(config, metadata)
		if err != nil {
			errors[metadata] = err
		}
	}

	if len(errors) != 0 {
		return &TelemetryMultiSyncError{Errors: errors}
	}

	return nil
}

// // RecorderMetricMultiple struct for metric sync
// type RecorderMetricMultiple struct {
// 	metricRecorder          service.MetricsRecorder
// 	metricsWrapper          *storage.MetricWrapper
// 	metricsJobsWaitingGroup sync.WaitGroup
// 	logger                  logging.LoggerInterface
// }
//
// // NewMetricRecorderMultiple creates new metric synchronizer for posting metrics
// func NewMetricRecorderMultiple(
// 	metricsWrapper *storage.MetricWrapper,
// 	metricRecorder service.MetricsRecorder,
// 	logger logging.LoggerInterface,
// ) metric.MetricRecorder {
// 	return &RecorderMetricMultiple{
// 		metricRecorder:          metricRecorder,
// 		metricsWrapper:          metricsWrapper,
// 		metricsJobsWaitingGroup: sync.WaitGroup{},
// 		logger:                  logger,
// 	}
// }
//
// func (r *RecorderMetricMultiple) sendLatencies() {
// 	// Decrement the counter when the goroutine completes.
// 	defer r.metricsJobsWaitingGroup.Done()
// 	latenciesToSend, err := r.metricsWrapper.Telemetry.PopLatenciesWithMetadata()
// 	if err != nil {
// 		r.logger.Error(err.Error())
// 	} else {
// 		latenciesToSend.ForEach(func(sdk string, ip string, latencies map[string][]int64) {
// 			latenciesDataSet := make([]dtos.LatenciesDTO, 0)
// 			for name, buckets := range latencies {
// 				latenciesDataSet = append(latenciesDataSet, dtos.LatenciesDTO{MetricName: name, Latencies: buckets})
// 			}
// 			if len(latenciesDataSet) > 0 {
// 				r.metricRecorder.RecordLatencies(latenciesDataSet, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
// 			}
// 		})
// 	}
// }
//
// func (r *RecorderMetricMultiple) sendCounters() {
// 	// Decrement the counter when the goroutine completes.
// 	defer r.metricsJobsWaitingGroup.Done()
//
// 	countersToSend, err := r.metricsWrapper.Telemetry.PopCountersWithMetadata()
// 	if err != nil {
// 		r.logger.Error(err.Error())
// 	} else {
// 		countersToSend.ForEach(func(sdk string, ip string, counters map[string]int64) {
// 			countersDataSet := make([]dtos.CounterDTO, 0)
// 			for metricName, count := range counters {
// 				countersDataSet = append(countersDataSet, dtos.CounterDTO{MetricName: metricName, Count: count})
// 			}
// 			if len(countersDataSet) > 0 {
// 				r.metricRecorder.RecordCounters(countersDataSet, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
// 			}
// 		})
// 	}
// }
//
// func (r *RecorderMetricMultiple) sendGauges() {
// 	// Decrement the counter when the goroutine completes.
// 	defer r.metricsJobsWaitingGroup.Done()
//
// 	gaugesToSend, err := r.metricsWrapper.Telemetry.PopGaugesWithMetadata()
// 	if err != nil {
// 		r.logger.Error(err.Error())
// 	} else {
// 		gaugesToSend.ForEach(func(sdk string, ip string, metricName string, value float64) {
// 			r.logger.Debug("Posting gauge:", metricName, value)
// 			r.metricRecorder.RecordGauge(dtos.GaugeDTO{MetricName: metricName, Gauge: value}, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
// 		})
// 	}
// }
//
// // SynchronizeTelemetry syncs metrics
// func (r *RecorderMetricMultiple) SynchronizeTelemetry() error {
// 	r.metricsJobsWaitingGroup.Add(3)
// 	go r.sendLatencies()
// 	go r.sendCounters()
// 	go r.sendGauges()
// 	r.metricsJobsWaitingGroup.Wait()
// 	return nil
// }
