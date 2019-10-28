// Package redis implements different kind of storages for split information
package redis

import (
	"fmt"
	"testing"
)

func TestPrefix(t *testing.T) {
	prefix := "some_prefix"
	prefixAdapter1 := &prefixAdapter{prefix: prefix}
	prefixTest := prefixAdapter1.setPrefixPattern("some_pattern")
	if prefixTest != fmt.Sprintf("%s.some_pattern", prefix) {
		t.Error("WITH PREFIX: Set prefix pattern mal-formed")
	}

	prefixAdapter2 := &prefixAdapter{prefix: ""}
	prefixTest2 := prefixAdapter2.setPrefixPattern("some_pattern")
	if prefixTest2 != fmt.Sprint("some_pattern") {
		t.Error("WITHOUT PREFIX: Set prefix pattern mal-formed")
	}
}

func TestNamespaces(t *testing.T) {

	languageAndVersion := "test-2.0"
	instanceID := "127.0.0.1"
	featureName := "some_feature"
	metricName := "some_metric"
	bucketNumber := "1"
	segmentName := "some_segment"

	prefixAdapter := &prefixAdapter{prefix: ""}

	impressionsKey := prefixAdapter.impressionsNamespace(languageAndVersion, instanceID, featureName)
	if impressionsKey != fmt.Sprintf("SPLITIO/%s/%s/impressions.%s", languageAndVersion, instanceID, featureName) {
		t.Error("Impressions Namespace mal-formed")
	}

	metricsCounterNamespace := prefixAdapter.metricsCounterNamespace(languageAndVersion, instanceID, metricName)
	if metricsCounterNamespace != fmt.Sprintf("SPLITIO/%s/%s/count.%s", languageAndVersion, instanceID, metricName) {
		t.Error("Metrics Counter namespace mal-formed")
	}

	metricsGaugeNamespace := prefixAdapter.metricsGaugeNamespace(languageAndVersion, instanceID, metricName)
	if metricsGaugeNamespace != fmt.Sprintf("SPLITIO/%s/%s/gauge.%s", languageAndVersion, instanceID, metricName) {
		t.Error("Metrics Gauges namespace mal-formed")
	}

	metricsLatencyNamespace := prefixAdapter.metricsLatencyNamespace(languageAndVersion, instanceID, metricName, bucketNumber)
	if metricsLatencyNamespace != fmt.Sprintf("SPLITIO/%s/%s/latency.%s.bucket.%s", languageAndVersion, instanceID, metricName, bucketNumber) {
		t.Error("Metrics Latency namespace mal-formed")
	}

	segmentNamespace := prefixAdapter.segmentNamespace(segmentName)
	if segmentNamespace != fmt.Sprintf("SPLITIO.segment.%s", segmentName) {
		t.Error("Segment namespace mal-formed")
	}

	segmentTillNamespace := prefixAdapter.segmentTillNamespace(segmentName)
	if segmentTillNamespace != fmt.Sprintf("SPLITIO.segment.%s.till", segmentName) {
		t.Error("Segment TILL namespace mal-formed")
	}

	segmentsRegisteredNamespace := prefixAdapter.segmentsRegisteredNamespace()
	if segmentsRegisteredNamespace != "SPLITIO.segments.registered" {
		t.Error("Registered Segments namespace mal-formed")
	}

	splitNamespace := prefixAdapter.splitNamespace(featureName)
	if splitNamespace != fmt.Sprintf("SPLITIO.split.%s", featureName) {
		t.Error("Split namespace mal-formed")
	}

	splitTillNamespace := prefixAdapter.splitsTillNamespace()
	if splitTillNamespace != "SPLITIO.splits.till" {
		t.Error("Split till namespace mal-formed")
	}

	trafficTypeNamespace := prefixAdapter.trafficTypeNamespace("tt")
	if trafficTypeNamespace != "SPLITIO.trafficType.tt" {
		t.Error("Traffic Type namespace mal-formed")
	}

	hashNamespace := prefixAdapter.hashNamespace()
	if hashNamespace != "SPLITIO.hash" {
		t.Error("APIKEY hash namespace malformed.")
	}
}
