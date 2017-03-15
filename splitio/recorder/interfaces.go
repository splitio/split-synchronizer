// Package recorder implements all kind of data recorders just like impressions and metrics
package recorder

import "github.com/splitio/go-agent/splitio/api"

// ImpressionsRecorder interface to be implemented by Impressions loggers
type ImpressionsRecorder interface {
	Post(impressions []api.ImpressionsDTO, sdkVersion string, machineIP string) error
}

// MetricsRecorder interface to be implemented by Metrics loggers
type MetricsRecorder interface {
	PostLatencies(latencies []api.LatenciesDTO, sdkVersion string, machineIP string) error
	PostCounters(counters []api.CounterDTO, sdkVersion string, machineIP string) error
	PostGauge(gauge api.GaugeDTO, sdkVersion string, machineIP string) error
}
