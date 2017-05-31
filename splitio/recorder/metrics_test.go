// Package recorder implements all kind of data recorders just like impressions and metrics
package recorder

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

func TestMetricsHTTPRecorderPostLatencies(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var latenciesInPost []api.LatenciesDTO
		err := json.Unmarshal(rBody, &latenciesInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if latenciesInPost[0].MetricName != "some_metric_name" ||
			latenciesInPost[0].Latencies[5] != 1234567890 {
			t.Error("Latencies arrived mal-formed")
		}

		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var latencyValues = make([]int64, 23) //23 maximun number of buckets
	latencyValues[5] = 1234567890
	var latenciesDataSet []api.LatenciesDTO
	latenciesDataSet = append(latenciesDataSet, api.LatenciesDTO{MetricName: "some_metric_name", Latencies: latencyValues})

	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostLatencies(latenciesDataSet, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestMetricsHTTPRecorderPostLatenciesHTTPError(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var latenciesDataSet []api.LatenciesDTO
	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostLatencies(latenciesDataSet, "test-1.0.0", "127.0.0.1")
	if err2 == nil {
		t.Error(err2)
	}
}

func TestMetricsHTTPRecorderPostCounters(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var countersInPost []api.CounterDTO
		err := json.Unmarshal(rBody, &countersInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if countersInPost[0].MetricName != "counter_1" ||
			countersInPost[0].Count != 111 ||
			countersInPost[1].MetricName != "counter_2" ||
			countersInPost[1].Count != 222 {
			t.Error("Counters arrived mal-formed")
		}

		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var countersDataSet []api.CounterDTO
	countersDataSet = append(countersDataSet, api.CounterDTO{MetricName: "counter_1", Count: 111}, api.CounterDTO{MetricName: "counter_2", Count: 222})

	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostCounters(countersDataSet, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestMetricsHTTPRecorderPostCountersHTTPError(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var countersDataSet []api.CounterDTO

	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostCounters(countersDataSet, "test-1.0.0", "127.0.0.1")
	if err2 == nil {
		t.Error(err2)
	}
}

func TestMetricsHTTPRecorderPostGauges(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var gaugesInPost api.GaugeDTO
		err := json.Unmarshal(rBody, &gaugesInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if gaugesInPost.MetricName != "gauge_1" ||
			gaugesInPost.Gauge != 111.1 {
			t.Error("Gauges arrived mal-formed")
		}

		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var gaugeDataSet api.GaugeDTO
	gaugeDataSet = api.GaugeDTO{MetricName: "gauge_1", Gauge: 111.1}

	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostGauge(gaugeDataSet, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestMetricsHTTPRecorderPostGaugesHTTPError(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	var gaugeDataSet api.GaugeDTO

	metricsHTTPRecorder := MetricsHTTPRecorder{}

	err2 := metricsHTTPRecorder.PostGauge(gaugeDataSet, "test-1.0.0", "127.0.0.1")
	if err2 == nil {
		t.Error(err2)
	}
}
