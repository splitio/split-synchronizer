package redis

import (
	"fmt"
	"strings"
)

//SplitNames
const _splitKeysNamespace = "SPLITIO.splitNames"

//Splits
const _splitNamespace = "SPLITIO.split.%s"
const _splitsTillNamespace = "SPLITIO.splits.till"

//Segments
const _segmentsRegisteredNamespace = "SPLITIO.segments.registered"
const _segmentTillNamespace = "SPLITIO.segment.%s.till"
const _segmentNamespace = "SPLITIO.segment.%s"

//Impressions

const _impressionKeysNamespace = "SPLITIO.impressionKeys"

//SPLITIO/{sdk-language-version}/{instance-id}/impressions.{featureName}
const _impressionsNamespace = "SPLITIO/%s/%s/impressions.%s"

//Metrics
const _metricsLatencyKeysNamespace = "SPLITIO.latencyNames"
const _metricsCountKeysNamespace = "SPLITIO.countNames"
const _metricsGaugeKeysNamespace = "SPLITIO.gaugeNames"

//SPLITIO/{sdk-language-version}/{instance-id}/latency.{metricName}.bucket.{bucketNumber}
const _metricsLatencyNamespace = "SPLITIO/%s/%s/latency.%s.bucket.%s"

//SPLITIO/{sdk-language-version}/{instance-id}/count.{metricName}
const _metricsCounterNamespace = "SPLITIO/%s/%s/count.%s"

//SPLITIO/{sdk-language-version}/{instance-id}/gauge.{metricName}
const _metricsGaugesNamespace = "SPLITIO/%s/%s/gauge.%s"

//Events
const _eventsListNamespace = "SPLITIO.events"

type prefixAdapter struct {
	prefix string
}

func (p prefixAdapter) setPrefixPattern(pattern string) string {
	if p.prefix != "" {
		return strings.Join([]string{p.prefix, pattern}, ".")
	}
	return pattern
}

func (p prefixAdapter) splitKeysNamespace() string {
	return p.setPrefixPattern(_splitKeysNamespace)
}

func (p prefixAdapter) splitNamespace(name string) string {
	return fmt.Sprintf(p.setPrefixPattern(_splitNamespace), name)
}

func (p prefixAdapter) splitsTillNamespace() string {
	return fmt.Sprint(p.setPrefixPattern(_splitsTillNamespace))
}

func (p prefixAdapter) segmentsRegisteredNamespace() string {
	return fmt.Sprint(p.setPrefixPattern(_segmentsRegisteredNamespace))
}

func (p prefixAdapter) segmentTillNamespace(name string) string {
	return fmt.Sprintf(p.setPrefixPattern(_segmentTillNamespace), name)
}

func (p prefixAdapter) segmentNamespace(name string) string {
	return fmt.Sprintf(p.setPrefixPattern(_segmentNamespace), name)
}

func (p prefixAdapter) impressionKeysNamespace() string {
	return p.setPrefixPattern(_impressionKeysNamespace)
}

func (p prefixAdapter) restoreImpressionKey(partial string) string {
	return p.setPrefixPattern("SPLITIO/" + partial)
}

func (p prefixAdapter) metricsLatencyKeys() string {
	return p.setPrefixPattern(_metricsLatencyKeysNamespace)
}

func (p prefixAdapter) metricsCounterKeys() string {
	return p.setPrefixPattern(_metricsCountKeysNamespace)
}

func (p prefixAdapter) metricsGaugeKeys() string {
	return p.setPrefixPattern(_metricsGaugeKeysNamespace)
}

func (p prefixAdapter) restoreMetricKey(partial string) string {
	return p.setPrefixPattern("SPLITIO/" + partial)
}

func (p prefixAdapter) impressionsNamespace(languageAndVersion string, instanceID string, featureName string) string {
	return fmt.Sprintf(p.setPrefixPattern(_impressionsNamespace), languageAndVersion, instanceID, featureName)
}

func (p prefixAdapter) metricsLatencyNamespace(languageAndVersion string, instanceID string, metricName string, bucketNumber string) string {
	return fmt.Sprintf(p.setPrefixPattern(_metricsLatencyNamespace), languageAndVersion, instanceID, metricName, bucketNumber)
}

func (p prefixAdapter) metricsCounterNamespace(languageAndVersion string, instanceID string, metricName string) string {
	return fmt.Sprintf(p.setPrefixPattern(_metricsCounterNamespace), languageAndVersion, instanceID, metricName)
}

func (p prefixAdapter) metricsGaugeNamespace(languageAndVersion string, instanceID string, metricName string) string {
	return fmt.Sprintf(p.setPrefixPattern(_metricsGaugesNamespace), languageAndVersion, instanceID, metricName)
}

func (p prefixAdapter) eventsListNamespace() string {
	return fmt.Sprint(p.setPrefixPattern(_eventsListNamespace))
}
