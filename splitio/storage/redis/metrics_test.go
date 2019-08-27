// Package redis implements different kind of storages for split information
package redis

import (
	"io/ioutil"
	//"strconv"
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

	prefixAdapter := &prefixAdapter{prefix: "metricstest"}

	languageAndVersion := "test-2.0"
	instanceID := "127.0.0.1"
	metricName := "some_metric"
	bucketNumber := "4"

	metricsStorageAdapter := NewMetricsStorageAdapter(Client, "metricstest")

	/* Metric Counters */
	counterKey := prefixAdapter.metricsCounterNamespace(languageAndVersion, instanceID, metricName)
	Client.Set(counterKey, 5, 0)
	if Client.Exists(counterKey).Val() != 1 {
		t.Error("Counter key should be present.")
	}

	retrievedCounters, err := metricsStorageAdapter.RetrieveCounters()
	if err != nil {
		t.Error(err)
	}

	executions := 0
	retrievedCounters.ForEach(func(sdk string, ip string, metrics map[string]int64) {
		if sdk != languageAndVersion {
			t.Error("Wrong SDK language/version")
		}

		if ip != instanceID {
			t.Error("Wrong IP Address")
		}

		value, ok := metrics[metricName]
		if !ok {
			t.Error("Wrong metric name")
		}

		if value != 5 {
			t.Error("Wrong metric value")
		}
		executions++
	})
	if executions != 1 {
		t.Error("Should have run once for counters")
	}

	/* Metric Gauges */
	gaugeKey := prefixAdapter.metricsGaugeNamespace(languageAndVersion, instanceID, metricName)
	Client.Set(gaugeKey, 3.24, 0)
	if Client.Exists(gaugeKey).Val() != 1 {
		t.Error("Gaguge key should be present")
	}

	retrievedGauges, errg := metricsStorageAdapter.RetrieveGauges()
	if errg != nil {
		t.Error(errg)
	}

	executions = 0
	retrievedGauges.ForEach(func(sdk string, ip string, name string, value float64) {
		if sdk != languageAndVersion {
			t.Error("Wrong SDK language/version")
		}

		if ip != instanceID {
			t.Error("Wrong IP Address")
		}

		if name != metricName {
			t.Error("Wrong name")
		}

		if value != 3.24 {
			t.Error("Wrong value")
		}
		executions++
	})
	if executions != 1 {
		t.Error("Should have run once for gauges")
	}

	/* Metric Latencies */
	latencyKey := prefixAdapter.metricsLatencyNamespace(languageAndVersion, instanceID, metricName, bucketNumber)
	Client.Set(latencyKey, 1234, 0)
	if Client.Exists(latencyKey).Val() != 1 {
		t.Error("Latency key should be present")
	}

	retrievedLatencies, errl := metricsStorageAdapter.RetrieveLatencies()
	if errl != nil {
		t.Error(errl)
	}

	executions = 0
	retrievedLatencies.ForEach(func(sdk string, ip string, metrics map[string][]int64) {
		if sdk != languageAndVersion {
			t.Error("Wrong SDK language/version")
		}

		if ip != instanceID {
			t.Error("Wrong IP Address")
		}

		buckets, ok := metrics[metricName]
		if !ok {
			t.Error("Wrong metric name")
		}

		if buckets[4] != 1234 {
			t.Error("Wrong metric value")
		}
		executions++
	})
	if executions != 1 {
		t.Error("Should have run once for latencies")
	}

	// Assert that the keys no longer exist
	if Client.Exists(latencyKey).Val() != 0 {
		t.Error("Latency key should have been removed!")
	}

	if Client.Exists(counterKey).Val() != 0 {
		t.Error("Latency key should have been removed!")
	}

	if Client.Exists(gaugeKey).Val() != 0 {
		t.Error("Latency key should have been removed!")
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
		if sdk != "" {
			t.Errorf("Sdk should be nil. Is %s", sdk)
		}
		if ip != "" {
			t.Errorf("Ip should be nil. Is %s", ip)
		}
		if feature != "" {
			t.Errorf("Feature should be nil. Is %s", feature)
		}
		if bucket != 0 {
			t.Errorf("Bucket should be nil. Is %d", bucket)
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
		if sdk != "" {
			t.Errorf("Sdk should be nil. Is %s", sdk)
		}
		if ip != "" {
			t.Errorf("Ip should be nil. Is %s", ip)
		}
		if feature != "" {
			t.Errorf("Feature should be nil. Is %s", feature)
		}
		if bucket != 0 {
			t.Errorf("Bucket should be nil. Is %d", bucket)
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
		if sdk != "" {
			t.Errorf("Sdk should be nil. Is %s", sdk)
		}
		if ip != "" {
			t.Errorf("Ip should be nil. Is %s", ip)
		}
		if feature != "" {
			t.Errorf("Feature should be nil. Is %s", feature)
		}
		if bucket != 0 {
			t.Errorf("Bucket should be nil. Is %d", bucket)
		}
	}
}
