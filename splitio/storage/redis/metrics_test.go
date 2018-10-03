// Package redis implements different kind of storages for split information
package redis

import (
	"io/ioutil"
	"strconv"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

func TestMetricsRedisStorageAdapter(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	prefixAdapter := &prefixAdapter{prefix: ""}

	languageAndVersion := "test-2.0"
	instanceID := "127.0.0.1"
	metricName := "some_metric"
	bucketNumber := "4"

	metricsStorageAdapter := NewMetricsStorageAdapter(Client, "")

	/* Metric Counters */
	counterKey := prefixAdapter.metricsCounterNamespace(languageAndVersion, instanceID, metricName)
	Client.Set(counterKey, 5, 0)

	retrievedCounters, err := metricsStorageAdapter.RetrieveCounters()
	if err != nil {
		t.Error(err)
	}

	_, ok1 := retrievedCounters[languageAndVersion]
	if !ok1 {
		t.Error("Error retrieving counters by language and version")
	}
	_, ok2 := retrievedCounters[languageAndVersion][instanceID]
	if !ok2 {
		t.Error("Error retrieving counters by instance ID ")
	}
	_, ok3 := retrievedCounters[languageAndVersion][instanceID][metricName]
	if !ok3 {
		t.Error("Error retrieving counter by name ")
	}

	if retrievedCounters[languageAndVersion][instanceID][metricName] != 5 {
		t.Error("Error retrieving counter value")
	}

	/* Metric Gauges */
	gaugeKey := prefixAdapter.metricsGaugeNamespace(languageAndVersion, instanceID, metricName)
	Client.Set(gaugeKey, 3.24, 0)

	retrievedGauges, errg := metricsStorageAdapter.RetrieveGauges()
	if errg != nil {
		t.Error(errg)
	}

	_, ok1 = retrievedGauges[languageAndVersion]
	if !ok1 {
		t.Error("Error retrieving gauges by language and version")
	}
	_, ok2 = retrievedGauges[languageAndVersion][instanceID]
	if !ok2 {
		t.Error("Error retrieving gauges by instance ID ")
	}
	_, ok3 = retrievedGauges[languageAndVersion][instanceID][metricName]
	if !ok3 {
		t.Error("Error retrieving gauges by name ")
	}

	if retrievedGauges[languageAndVersion][instanceID][metricName] != 3.24 {
		t.Error("Error retrieving gauge value")
	}

	/* Metric Latencies */
	latencyKey := prefixAdapter.metricsLatencyNamespace(languageAndVersion, instanceID, metricName, bucketNumber)
	Client.Set(latencyKey, 1234, 0)

	retrievedLatencies, errl := metricsStorageAdapter.RetrieveLatencies()
	if errl != nil {
		t.Error(errl)
	}

	_, ok1 = retrievedLatencies[languageAndVersion]
	if !ok1 {
		t.Error("Error retrieving latencies by language and version")
	}
	_, ok2 = retrievedLatencies[languageAndVersion][instanceID]
	if !ok2 {
		t.Error("Error retrieving latencies by instance ID ")
	}
	_, ok3 = retrievedLatencies[languageAndVersion][instanceID][metricName]
	if !ok3 {
		t.Error("Error retrieving latencies by name ")
	}
	bucketNumberInt, _ := strconv.Atoi(bucketNumber)
	if retrievedLatencies[languageAndVersion][instanceID][metricName][bucketNumberInt] != 1234 {
		t.Error("Error retrieving latencie value")
	}

}

func TestThatMalformedLatencyKeysDoNotPanic(t *testing.T) {
	wrongKeys := []string{
		"SPLITIO/php-5.3.1//latency.sdk.get_treatment.bucket.15",
		"SPLITIO//123.123.123.123/latency.sdk.get_treatment.bucket.15",
		"SPLITIO///latency.sdk.get_treatment.bucket.s15",
		"SPLITIO//////.sdk.get_treatment.bucket.15",
		"/php-5.3.1/123.123.123.123/latency.sdk.get_treatment.bucket.15",
	}

	for _, key := range wrongKeys {
		sdk, ip, feature, bucket, err := parseLatencyKey(key)
		if err == nil {
			t.Error("An error should have been returned.")
		}
		if sdk != nil {
			t.Errorf("Sdk should be nil. Is %s", *sdk)
		}
		if ip != nil {
			t.Errorf("Ip should be nil. Is %s", *ip)
		}
		if feature != nil {
			t.Errorf("Feature should be nil. Is %s", *feature)
		}
		if bucket != nil {
			t.Errorf("Bucket should be nil. Is %d", *bucket)
		}
	}
}

func TestThatMalformedCounterKeysDoNotPanic(t *testing.T) {
	wrongKeys := []string{
		"SPLITIO/php-5.3.1//count.http_errors",
		"SPLITIO//123.123.123.123/count.http_errors",
		"SPLITIO///count.http_errors",
		"SPLITIO//////count.http_errors",
		"/php-5.3.1/123.123.123.123/count.http_errors",
	}

	for _, key := range wrongKeys {
		sdk, ip, feature, bucket, err := parseLatencyKey(key)
		if err == nil {
			t.Error("An error should have been returned.")
		}
		if sdk != nil {
			t.Errorf("Sdk should be nil. Is %s", *sdk)
		}
		if ip != nil {
			t.Errorf("Ip should be nil. Is %s", *ip)
		}
		if feature != nil {
			t.Errorf("Feature should be nil. Is %s", *feature)
		}
		if bucket != nil {
			t.Errorf("Bucket should be nil. Is %d", *bucket)
		}
	}
}

func TestThatMalformedGaugeKeysDoNotPanic(t *testing.T) {
	wrongKeys := []string{
		"SPLITIO/php-5.3.1//gauge.storage_fill_percentage",
		"SPLITIO//123.123.123.123/gauge.storage_fill_percentage",
		"SPLITIO///gauge.storage_fill_percentage",
		"SPLITIO//////gauge.storage_fill_percentage",
		"/php-5.3.1/123.123.123.123/gauge.storage_fill_percentage",
	}

	for _, key := range wrongKeys {
		sdk, ip, feature, bucket, err := parseLatencyKey(key)
		if err == nil {
			t.Error("An error should have been returned.")
		}
		if sdk != nil {
			t.Errorf("Sdk should be nil. Is %s", *sdk)
		}
		if ip != nil {
			t.Errorf("Ip should be nil. Is %s", *ip)
		}
		if feature != nil {
			t.Errorf("Feature should be nil. Is %s", *feature)
		}
		if bucket != nil {
			t.Errorf("Bucket should be nil. Is %d", *bucket)
		}
	}
}
