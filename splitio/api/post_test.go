// Package api contains all functions and dtos Split APIs
package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/go-agent/log"
)

func TestPostImpressions(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "ip-127-0-0-1" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		//fmt.Println(string(rBody))
		var impressionsInPost []ImpressionsDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if impressionsInPost[0].TestName != "some_test" ||
			impressionsInPost[0].KeyImpressions[0].KeyName != "some_key_1" ||
			impressionsInPost[0].KeyImpressions[1].KeyName != "some_key_2" {
			t.Error("Posted impressions arrived mal-formed")
		}

		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	imp1 := ImpressionDTO{KeyName: "some_key_1", Treatment: "on", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_1", BucketingKey: "some_bucket_key_1"}
	imp2 := ImpressionDTO{KeyName: "some_key_2", Treatment: "off", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_2", BucketingKey: "some_bucket_key_2"}

	keyImpressions := make([]ImpressionDTO, 0)
	keyImpressions = append(keyImpressions, imp1, imp2)
	impressionsTest := ImpressionsDTO{TestName: "some_test", KeyImpressions: keyImpressions}

	impressions := make([]ImpressionsDTO, 0)
	impressions = append(impressions, impressionsTest)

	data, err := json.Marshal(impressions)
	if err != nil {
		t.Error(err)
		return
	}

	err2 := PostImpressions(data, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestPostMetricsLatency(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "ip-127-0-0-1" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var latenciesInPost []LatenciesDTO
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

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	var latencyValues = make([]int64, 23) //23 maximun number of buckets
	latencyValues[5] = 1234567890
	var latenciesDataSet []LatenciesDTO
	latenciesDataSet = append(latenciesDataSet, LatenciesDTO{MetricName: "some_metric_name", Latencies: latencyValues})

	data, err := json.Marshal(latenciesDataSet)
	if err != nil {
		t.Error(err)
		return
	}

	err2 := PostMetricsLatency(data, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestPostMetricsCounters(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "ip-127-0-0-1" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var countersInPost []CounterDTO
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

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	var countersDataSet []CounterDTO
	countersDataSet = append(countersDataSet, CounterDTO{MetricName: "counter_1", Count: 111}, CounterDTO{MetricName: "counter_2", Count: 222})

	data, err := json.Marshal(countersDataSet)
	if err != nil {
		t.Error(err)
		return
	}

	err2 := PostMetricsCounters(data, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}
}

func TestPostMetricsGauge(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "ip-127-0-0-1" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var gaugesInPost GaugeDTO
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

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	var gaugeDataSet GaugeDTO
	gaugeDataSet = GaugeDTO{MetricName: "gauge_1", Gauge: 111.1}

	data, err := json.Marshal(gaugeDataSet)
	if err != nil {
		t.Error(err)
		return
	}

	err2 := PostMetricsGauge(data, "test-1.0.0", "127.0.0.1")
	if err2 != nil {
		t.Error(err2)
	}

}
