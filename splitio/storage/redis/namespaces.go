package redis

import (
	"fmt"
	"strings"
)

//Splits
const _splitNamespace = "SPLITIO.split.%s"
const _splitsTillNamespace = "SPLITIO.splits.till"

//Segments
const _segmentsRegisteredNamespace = "SPLITIO.segments.registered"
const _segmentTillNamespace = "SPLITIO.segment.%s.till"
const _segmentNamespace = "SPLITIO.segment.%s"

//Impressions
//SPLITIO/{sdk-language-version}/{instance-id}/impressions.{featureName}
const _impressionsNamespace = "SPLITIO/%s/%s/impressions.%s"

//Metrics
//SPLITIO/{sdk-language-version}/{instance-id}/latency.{metricName}.bucket.{bucketNumber}
const _metricsLatencyNamespace = "SPLITIO/%s/%s/latency.%s.bucket.%s"

//SPLITIO/{sdk-language-version}/{instance-id}/count.{metricName}
const _metricsCounterNamespace = "SPLITIO/%s/%s/count.%s"

//SPLITIO/{sdk-language-version}/{instance-id}/gauge.{metricName}
const _metricsGaugesNamespace = "SPLITIO/%s/%s/gauge.%s"

//Events
const _eventsListNamespace = "SPLITIO.events"

const _impressionsQueueNamespace = "SPLITIO.impressions"

type prefixAdapter struct {
	prefix string
}

func (p prefixAdapter) setPrefixPattern(pattern string) string {
	if p.prefix != "" {
		return strings.Join([]string{p.prefix, pattern}, ".")
	}
	return pattern
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

func (p prefixAdapter) impressionsQueueNamespace() string {
	return fmt.Sprint(p.setPrefixPattern(_impressionsQueueNamespace))
}
