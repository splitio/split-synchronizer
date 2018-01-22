package api

import (
	"fmt"
	"strings"
)

func postToEventsServer(url string, data []byte, sdkVersion string, machineIP string, machineName string) error {
	var _client = *eventsClient
	_client.ResetHeaders()
	_client.AddHeader("SplitSDKVersion", sdkVersion)
	_client.AddHeader("SplitSDKMachineIP", machineIP)
	if machineName == "" && machineIP != "" {
		_client.AddHeader("SplitSDKMachineName", fmt.Sprintf("ip-%s", strings.Replace(machineIP, ".", "-", -1)))
	} else {
		_client.AddHeader("SplitSDKMachineName", machineName)
	}

	err := _client.Post(url, data)
	if err != nil {
		return err
	}
	return nil
}

func postMetrics(url string, data []byte, sdkVersion string, machineIP string) error {

	return postToEventsServer(url, data, sdkVersion, machineIP, "")
}

// PostImpressions send impressions to Split events service
func PostImpressions(data []byte, sdkVersion string, machineIP string, machineName string) error {

	url := "/testImpressions/bulk"

	return postToEventsServer(url, data, sdkVersion, machineIP, machineName)
}

// PostMetricsLatency send latencies to Split events service.
func PostMetricsLatency(data []byte, sdkVersion string, machineIP string) error {
	url := "/metrics/times"
	return postMetrics(url, data, sdkVersion, machineIP)
}

// PostMetricsCounters send counts to Split events service.
func PostMetricsCounters(data []byte, sdkVersion string, machineIP string) error {
	url := "/metrics/counters"
	return postMetrics(url, data, sdkVersion, machineIP)
}

// PostMetricsGauge send counts to Split events service.
func PostMetricsGauge(data []byte, sdkVersion string, machineIP string) error {
	url := "/metrics/gauge"
	return postMetrics(url, data, sdkVersion, machineIP)
}

// PostMetricsCount send count to Split events service.
func PostMetricsCount(data []byte, sdkVersion string, machineIP string) error {
	url := "/metrics/counter"
	return postMetrics(url, data, sdkVersion, machineIP)
}

// PostMetricsTime send time latency to Split events service.
func PostMetricsTime(data []byte, sdkVersion string, machineIP string) error {
	url := "/metrics/time"
	return postMetrics(url, data, sdkVersion, machineIP)
}

// PostEvents send events to Split events service
func PostEvents(data []byte, sdkVersion string, machineIP string, machineName string) error {

	url := "/events/bulk"

	return postToEventsServer(url, data, sdkVersion, machineIP, machineName)
}
