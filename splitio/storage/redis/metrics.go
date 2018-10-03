package redis

import (
	"fmt"
	redis "gopkg.in/redis.v5"
	"regexp"
	"strconv"
	"strings"

	"github.com/splitio/split-synchronizer/log"
)

const maxBuckets = 23

// MetricsRedisStorageAdapter implements MetricsStorage interface
type MetricsRedisStorageAdapter struct {
	*BaseStorageAdapter
}

// NewMetricsStorageAdapter returns an instance of ImpressionStorageAdapter
func NewMetricsStorageAdapter(clientInstance *redis.Client, prefix string) *MetricsRedisStorageAdapter {
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

// RetrieveGauges returns gauges values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveGauges() (map[string]map[string]map[string]float64, error) {
	_keys, err := s.client.Keys(s.metricsGaugeNamespace("*", "*", "*")).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	gaugesToReturn := make(map[string]map[string]map[string]float64)
	for _, key := range _keys {
		sdkNameAndVersion, machineIP, metricName, err := parseMetricKey("gauge", key)
		if err != nil {
			log.Error.Printf("Unable to parse key %s. Skipping", key)
			s.client.Del(key)
			continue
		}
		value, err := s.client.GetSet(key, 0).Float64()
		if err != nil {
			log.Error.Println(err.Error())
			// continue next key
			continue
		}

		if _, ok := gaugesToReturn[sdkNameAndVersion]; !ok {
			gaugesToReturn[sdkNameAndVersion] = make(map[string]map[string]float64)
		}

		if _, ok := gaugesToReturn[sdkNameAndVersion][machineIP]; !ok {
			gaugesToReturn[sdkNameAndVersion][machineIP] = make(map[string]float64)
		}

		gaugesToReturn[sdkNameAndVersion][machineIP][metricName] = value

	}

	return gaugesToReturn, nil
}

// RetrieveCounters returns counter values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveCounters() (map[string]map[string]map[string]int64, error) {
	_keys, err := s.client.Keys(s.metricsCounterNamespace("*", "*", "*")).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	countersToReturn := make(map[string]map[string]map[string]int64)
	for _, key := range _keys {
		sdkNameAndVersion, machineIP, metricName, err := parseMetricKey("count", key)
		if err != nil {
			log.Error.Printf("Unable to parse key %s. Skipping", key)
			s.client.Del(key)
			continue
		}
		value, err := s.client.GetSet(key, 0).Int64()
		if err != nil {
			log.Error.Println(err.Error())
			// continue next key
			continue
		}

		if _, ok := countersToReturn[sdkNameAndVersion]; !ok {
			countersToReturn[sdkNameAndVersion] = make(map[string]map[string]int64)
		}

		if _, ok := countersToReturn[sdkNameAndVersion][machineIP]; !ok {
			countersToReturn[sdkNameAndVersion][machineIP] = make(map[string]int64)
		}

		countersToReturn[sdkNameAndVersion][machineIP][metricName] = value

	}

	return countersToReturn, nil
}

// RetrieveLatencies returns latency values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveLatencies() (map[string]map[string]map[string][]int64, error) {
	//(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/latency.([^\/]+).bucket.([0-9]*)

	_keys, err := s.client.Keys(s.metricsLatencyNamespace("*", "*", "*", "*")).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	// [sdkNameAndVersion][machineIP][metricName] = [0,0,0,0,0,0,0,0,0,0,0 ... ]
	latenciesToReturn := make(map[string]map[string]map[string][]int64)

	for _, key := range _keys {
		sdkNameAndVersion, machineIP, metricName, bucketNumber, err := parseLatencyKey(key)
		if err != nil {
			log.Warning.Printf("Unable to parse key %s. Removing it", key)
			s.client.Del(key)
			continue
		}

		value, err := s.client.GetSet(key, 0).Int64()
		if err != nil {
			log.Error.Println(err.Error())
			// continue next key
			continue
		}

		if _, ok := latenciesToReturn[sdkNameAndVersion]; !ok {
			latenciesToReturn[sdkNameAndVersion] = make(map[string]map[string][]int64)
		}

		if _, ok := latenciesToReturn[sdkNameAndVersion][machineIP]; !ok {
			latenciesToReturn[sdkNameAndVersion][machineIP] = make(map[string][]int64)
		}

		if _, ok := latenciesToReturn[sdkNameAndVersion][machineIP][metricName]; !ok {
			latenciesToReturn[sdkNameAndVersion][machineIP][metricName] = make([]int64, maxBuckets)
		}

		latenciesToReturn[sdkNameAndVersion][machineIP][metricName][bucketNumber] = value
	}
	log.Verbose.Println(latenciesToReturn)
	return latenciesToReturn, nil
}
