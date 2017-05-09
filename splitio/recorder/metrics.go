package recorder

import (
	"encoding/json"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// MetricsHTTPRecorder implrements ImpressionsRecorder interface
type MetricsHTTPRecorder struct{}

// PostLatencies posts metrics to HTTP Events server
func (r MetricsHTTPRecorder) PostLatencies(latencies []api.LatenciesDTO, sdkVersion string, machineIP string) error {

	log.Debug.Println("Posting Metrics for", sdkVersion, machineIP)

	data, err := json.Marshal(latencies)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostMetricsLatency(data, sdkVersion, machineIP); err != nil {
		log.Error.Println("Error posting metrics latency", err.Error())
		return err
	}

	return nil
}

// PostCounters posts metrics to HTTP Events server
func (r MetricsHTTPRecorder) PostCounters(counters []api.CounterDTO, sdkVersion string, machineIP string) error {

	log.Debug.Println("Posting Counter Metrics for", sdkVersion, machineIP)

	data, err := json.Marshal(counters)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostMetricsCounters(data, sdkVersion, machineIP); err != nil {
		log.Error.Println("Error posting metrics counter", err.Error())
		return err
	}

	return nil
}

// PostGauge posts metrics to HTTP Events server
func (r MetricsHTTPRecorder) PostGauge(gauge api.GaugeDTO, sdkVersion string, machineIP string) error {
	log.Debug.Println("Posting Gauges Metrics for", sdkVersion, machineIP)

	data, err := json.Marshal(gauge)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostMetricsGauge(data, sdkVersion, machineIP); err != nil {
		log.Error.Println("Error posting metrics gauge", err.Error())
		return err
	}

	return nil
}
