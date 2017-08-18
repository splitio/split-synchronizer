package redis

import (
	"regexp"
	"strconv"

	redis "gopkg.in/redis.v5"

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

// RetrieveGauges returns gauges values saved in Redis by SDKs
func (s MetricsRedisStorageAdapter) RetrieveGauges() (map[string]map[string]map[string]float64, error) {
	_keys, err := s.client.Keys(s.metricsGaugeNamespace("*", "*", "*")).Result()
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	gaugesToReturn := make(map[string]map[string]map[string]float64)
	for _, key := range _keys {
		var re = regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/gauge.([^\/]+)`)
		match := re.FindStringSubmatch(key)

		sdkNameAndVersion := match[2]
		machineIP := match[3]
		metricName := match[4]

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
		var re = regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/count.([^\/]+)`)
		match := re.FindStringSubmatch(key)

		sdkNameAndVersion := match[2]
		machineIP := match[3]
		metricName := match[4]

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
		var re = regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/latency.([^\/]+).bucket.([0-9]*)`)
		match := re.FindStringSubmatch(key)

		sdkNameAndVersion := match[2]
		machineIP := match[3]
		metricName := match[4]
		bucketNumber, _ := strconv.Atoi(match[5])

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
