package redis

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	redis "gopkg.in/redis.v5"
)

// ImpressionStorageAdapter implements ImpressionStorage interface
type ImpressionStorageAdapter struct {
	*BaseStorageAdapter
}

// NewImpressionStorageAdapter returns an instance of ImpressionStorageAdapter
func NewImpressionStorageAdapter(clientInstance *redis.Client, prefix string) *ImpressionStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := ImpressionStorageAdapter{adapter}
	return &client
}

// RetrieveImpressions returns cached impressions
func (r ImpressionStorageAdapter) RetrieveImpressions() (map[string]map[string][]api.ImpressionsDTO, error) {

	_keys, err := r.client.Keys(r.impressionsNamespace("*", "*", "*")).Result()
	if err == redis.Nil {
		log.Debug.Println("Fetching impression Keys", err.Error())
		return nil, nil
	} else if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	impressionsToReturn := make(map[string]map[string][]api.ImpressionsDTO)

	/*TODO see the edge case:
	impressionsPerPost < len(keys)
	*/
	log.Benchmark.Println("Impressions per post", conf.Data.ImpressionsPerPost)
	var impressionsPerKey = conf.Data.ImpressionsPerPost
	if len(_keys) > 0 && impressionsPerKey >= int64(len(_keys)) {
		impressionsPerKey = conf.Data.ImpressionsPerPost / int64(len(_keys))
	} else if impressionsPerKey < int64(len(_keys)) {
		// TODO add extra logic when impressionsPerKey is les than Key number in Redis
		impressionsPerKey = 1
	}

	log.Benchmark.Println("Number of Keys", len(_keys))
	log.Benchmark.Println("Impressions per key", impressionsPerKey)

	// To optimize the impressions retrieval process, we track the extra slots from features
	// that have less impressions than `impressionsPerKey`. In order to use those slots
	// for other features.
	var extraQuota int64
	for _, key := range _keys {
		log.Benchmark.Println("---", key)
		beforeSRandMemberN := time.Now().UnixNano()
		impressions, err := r.client.SRandMemberN(key, impressionsPerKey+extraQuota).Result()
		log.Benchmark.Println("SRandMemberN took", (time.Now().UnixNano() - beforeSRandMemberN))
		log.Verbose.Println(impressions)
		if err != nil {
			log.Debug.Println("Fetching impressions", err.Error())
			continue
		}
		extraQuota = (impressionsPerKey + extraQuota) - int64(len(impressions))
		if len(impressions) == 0 {
			log.Debug.Println("Not found impressions for this key", key)
			continue
		}

		var _keyImpressions []api.ImpressionDTO
		for _, impression := range impressions {
			var impressionDTO api.ImpressionDTO
			err = json.Unmarshal([]byte(impression), &impressionDTO)
			if err != nil {
				log.Warning.Println("The impression cannot be decoded from JSON", err.Error())
				log.Verbose.Println("Impression value:", impression)
				continue
			}
			_keyImpressions = append(_keyImpressions, impressionDTO)
		}

		//(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/impressions.([\s\S]*)
		var re = regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/impressions.([\s\S]*)`)
		match := re.FindStringSubmatch(key)

		sdkNameAndVersion := match[2]
		machineIP := match[3]
		featureName := match[4]

		log.Verbose.Println("Impression parsed key", match)

		if _, ok := impressionsToReturn[sdkNameAndVersion][machineIP]; !ok {
			impressionsToReturn[sdkNameAndVersion] = make(map[string][]api.ImpressionsDTO)
		}
		impressionsToReturn[sdkNameAndVersion][machineIP] = append(impressionsToReturn[sdkNameAndVersion][machineIP], api.ImpressionsDTO{TestName: featureName, KeyImpressions: _keyImpressions})

		//DELETE impressions
		_impressionsToDelete := make([]interface{}, len(impressions))
		for i, v := range impressions {
			_impressionsToDelete[i] = v
		}
		beforeSRem := time.Now().UnixNano()
		if err := r.client.SRem(key, _impressionsToDelete...).Err(); err != nil {
			log.Error.Println("Error removing impressions from Redis", err.Error())
			log.Verbose.Println(impressions)
		}
		log.Benchmark.Println("Srem took", (time.Now().UnixNano() - beforeSRem))
	}

	return impressionsToReturn, nil
}
