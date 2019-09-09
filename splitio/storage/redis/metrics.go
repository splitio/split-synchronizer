package redis

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-redis/redis"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

// MetricsRedisStorageAdapter implements MetricsStorage interface
type MetricsRedisStorageAdapter struct {
	*BaseStorageAdapter
}

// NewMetricsStorageAdapter returns an instance of ImpressionStorageAdapter
func NewMetricsStorageAdapter(clientInstance redis.UniversalClient, prefix string) *MetricsRedisStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := MetricsRedisStorageAdapter{adapter}
	return &client
}

func parseMetricKey(metricType string, key string) (string, string, string, error) {
	var re = regexp.MustCompile(strings.Replace(
		`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/{metricType}.([\s\S]*)`,
		"{metricType}",
		metricType,
		1,
	))
	match := re.FindStringSubmatch(key)

	if len(match) < 5 {
		return "", "", "", fmt.Errorf("Error parsing key %s", key)
	}

	sdkNameAndVersion := match[2]
	if sdkNameAndVersion == "" {
		return "", "", "", fmt.Errorf("Invalid sdk name/version")
	}

	machineIP := match[3]
	if machineIP == "" {
		return "", "", "", fmt.Errorf("Invalid machine IP")
	}

	metricName := match[4]
	if metricName == "" {
		return "", "", "", fmt.Errorf("Invalid feature name")
	}

	log.Verbose.Println("Impression parsed key", match)

	return sdkNameAndVersion, machineIP, metricName, nil
}

func parseLatencyKey(key string) (string, string, string, int, error) {
	re := regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/latency.([^\/]+).bucket.([0-9]*)`)
	match := re.FindStringSubmatch(key)

	if len(match) < 6 {
		return "", "", "", 0, fmt.Errorf("Error parsing key %s", key)
	}

	sdkNameAndVersion := match[2]
	if sdkNameAndVersion == "" {
		return "", "", "", 0, fmt.Errorf("Invalid sdk name/version")
	}

	machineIP := match[3]
	if machineIP == "" {
		return "", "", "", 0, fmt.Errorf("Invalid machine IP")
	}

	metricName := match[4]
	if metricName == "" {
		return "", "", "", 0, fmt.Errorf("Invalid feature name")
	}

	bucketNumber, err := strconv.Atoi(match[5])
	if err != nil {
		return "", "", "", 0, fmt.Errorf("Error parsing bucket number: %s", err.Error())
	}
	log.Verbose.Println("Impression parsed key", match)

	return sdkNameAndVersion, machineIP, metricName, bucketNumber, nil
}

func (s *MetricsRedisStorageAdapter) popByPattern(pattern string, useTransaction bool) (map[string]interface{}, error) {
	keys, err := s.client.Keys(pattern).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	if len(keys) == 0 {
		return map[string]interface{}{}, nil
	}

	values, err := s.client.MGet(keys...).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}
	_, err = s.client.Del(keys...).Result()
	if err != nil {
		// if we failed to delete the keys, log an error and continue working.
		log.Error.Println(err.Error())
	}

	toReturn := make(map[string]interface{})
	for index := range keys {
		if index >= len(keys) || index >= len(values) {
			break
		}
		toReturn[keys[index]] = values[index]
	}
	return toReturn, nil

}

func parseIntRedisValue(s interface{}) (int64, error) {
	asStr, ok := s.(string)
	if !ok {
		return 0, fmt.Errorf("%+v is not a string", s)
	}

	asInt64, err := strconv.ParseInt(asStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return asInt64, nil
}

func parseFloatRedisValue(s interface{}) (float64, error) {
	asStr, ok := s.(string)
	if !ok {
		return 0, fmt.Errorf("%+v is not a string", s)
	}

	asFloat64, err := strconv.ParseFloat(asStr, 64)
	if err != nil {
		return 0, err
	}

	return asFloat64, nil
}

// RetrieveGauges returns gauges values saved in Redis by SDKs
func (s *MetricsRedisStorageAdapter) RetrieveGauges() (*storage.GaugeDataBulk, error) {
	data, err := s.popByPattern(s.metricsGaugeNamespace("*", "*", "*"), false)
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	gaugesToReturn := storage.NewGaugeDataBulk()
	for key, value := range data {
		sdkNameAndVersion, machineIP, metricName, err := parseMetricKey("gauge", key)
		if err != nil {
			log.Error.Printf("Unable to parse key %s. Skipping", key)
			continue
		}
		asFloat, err := parseFloatRedisValue(value)
		if err != nil {
			log.Error.Printf("Unable to parse value %+v. Skipping", value)
			continue
		}
		gaugesToReturn.PutGauge(sdkNameAndVersion, machineIP, metricName, asFloat)
	}

	return gaugesToReturn, nil
}

// RetrieveCounters returns counter values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveCounters() (*storage.CounterDataBulk, error) {
	data, err := s.popByPattern(s.metricsCounterNamespace("*", "*", "*"), false)
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	countersToReturn := storage.NewCounterDataBulk()
	for key, value := range data {
		sdkNameAndVersion, machineIP, metricName, err := parseMetricKey("count", key)
		if err != nil {
			log.Error.Printf("Unable to parse key %s. Skipping", key)
			continue
		}
		asInt, err := parseIntRedisValue(value)
		if err != nil {
			log.Error.Println(err.Error())
			continue
		}

		countersToReturn.PutCounter(sdkNameAndVersion, machineIP, metricName, asInt)
	}

	return countersToReturn, nil
}

// RetrieveLatencies returns latency values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveLatencies() (*storage.LatencyDataBulk, error) {
	//(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/latency.([^\/]+).bucket.([0-9]*)

	data, err := s.popByPattern(s.metricsLatencyNamespace("*", "*", "*", "*"), false)
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	latenciesToReturn := storage.NewLatencyDataBulk()
	for key, value := range data {
		value, err := parseIntRedisValue(value)
		if err != nil {
			log.Warning.Printf("Unable to parse value of key %s. Skipping", key)
			continue
		}
		sdkNameAndVersion, machineIP, metricName, bucketNumber, err := parseLatencyKey(key)
		if err != nil {
			log.Warning.Printf("Unable to parse key %s. Skipping", key)
			continue
		}
		latenciesToReturn.PutLatency(sdkNameAndVersion, machineIP, metricName, bucketNumber, value)
	}
	log.Verbose.Println(latenciesToReturn)
	return latenciesToReturn, nil
}
