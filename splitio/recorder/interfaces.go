package recorder

import "github.com/splitio/split-synchronizer/splitio/api"

// ImpressionsRecorder interface to be implemented by Impressions loggers
type ImpressionsRecorder interface {
	Post(impressions []api.ImpressionsDTO, metadata api.SdkMetadata) error
}

// MetricsRecorder interface to be implemented by Metrics loggers
type MetricsRecorder interface {
	PostLatencies(latencies []api.LatenciesDTO, sdkVersion string, machineIP string) error
	PostCounters(counters []api.CounterDTO, sdkVersion string, machineIP string) error
	PostGauge(gauge api.GaugeDTO, sdkVersion string, machineIP string) error
}

// EventsRecorder interface to be implemented by Events loggers
type EventsRecorder interface {
	Post(events []api.EventDTO, sdkVersion string, machineIP string, machineName string) error
}
