package worker

import (
	"sync"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/service"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-split-commons/v2/synchronizer/worker/metric"
	"github.com/splitio/go-toolkit/v3/logging"
)

// RecorderMetricMultiple struct for metric sync
type RecorderMetricMultiple struct {
	metricRecorder          service.MetricsRecorder
	metricsWrapper          *storage.MetricWrapper
	metricsJobsWaitingGroup sync.WaitGroup
	logger                  logging.LoggerInterface
}

// NewMetricRecorderMultiple creates new metric synchronizer for posting metrics
func NewMetricRecorderMultiple(
	metricsWrapper *storage.MetricWrapper,
	metricRecorder service.MetricsRecorder,
	logger logging.LoggerInterface,
) metric.MetricRecorder {
	return &RecorderMetricMultiple{
		metricRecorder:          metricRecorder,
		metricsWrapper:          metricsWrapper,
		metricsJobsWaitingGroup: sync.WaitGroup{},
		logger:                  logger,
	}
}

func (r *RecorderMetricMultiple) sendLatencies() {
	// Decrement the counter when the goroutine completes.
	defer r.metricsJobsWaitingGroup.Done()
	latenciesToSend, err := r.metricsWrapper.Telemetry.PopLatenciesWithMetadata()
	if err != nil {
		r.logger.Error(err.Error())
	} else {
		latenciesToSend.ForEach(func(sdk string, ip string, latencies map[string][]int64) {
			latenciesDataSet := make([]dtos.LatenciesDTO, 0)
			for name, buckets := range latencies {
				latenciesDataSet = append(latenciesDataSet, dtos.LatenciesDTO{MetricName: name, Latencies: buckets})
			}
			if len(latenciesDataSet) > 0 {
				r.metricRecorder.RecordLatencies(latenciesDataSet, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
			}
		})
	}
}

func (r *RecorderMetricMultiple) sendCounters() {
	// Decrement the counter when the goroutine completes.
	defer r.metricsJobsWaitingGroup.Done()

	countersToSend, err := r.metricsWrapper.Telemetry.PopCountersWithMetadata()
	if err != nil {
		r.logger.Error(err.Error())
	} else {
		countersToSend.ForEach(func(sdk string, ip string, counters map[string]int64) {
			countersDataSet := make([]dtos.CounterDTO, 0)
			for metricName, count := range counters {
				countersDataSet = append(countersDataSet, dtos.CounterDTO{MetricName: metricName, Count: count})
			}
			if len(countersDataSet) > 0 {
				r.metricRecorder.RecordCounters(countersDataSet, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
			}
		})
	}
}

func (r *RecorderMetricMultiple) sendGauges() {
	// Decrement the counter when the goroutine completes.
	defer r.metricsJobsWaitingGroup.Done()

	gaugesToSend, err := r.metricsWrapper.Telemetry.PopGaugesWithMetadata()
	if err != nil {
		r.logger.Error(err.Error())
	} else {
		gaugesToSend.ForEach(func(sdk string, ip string, metricName string, value float64) {
			r.logger.Debug("Posting gauge:", metricName, value)
			r.metricRecorder.RecordGauge(dtos.GaugeDTO{MetricName: metricName, Gauge: value}, dtos.Metadata{MachineIP: ip, SDKVersion: sdk})
		})
	}
}

// SynchronizeTelemetry syncs metrics
func (r *RecorderMetricMultiple) SynchronizeTelemetry() error {
	r.metricsJobsWaitingGroup.Add(3)
	go r.sendLatencies()
	go r.sendCounters()
	go r.sendGauges()
	r.metricsJobsWaitingGroup.Wait()
	return nil
}
