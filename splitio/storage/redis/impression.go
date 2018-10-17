package redis

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"fmt"

	"github.com/go-redis/redis"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

var impressionKeysWithCardinalityScriptTemplate = `
	local impkeys = redis.call('KEYS', '{KEY_NAMESPACE}')
	local featureCardinality = {}
	for i, key in ipairs(impkeys) do
	    featureCardinality[2 * i - 1] = key
		featureCardinality[2 * i ] =  redis.call('SCARD', key)
	end
	return featureCardinality`

/*
Private types and functions to handle list of pairs (impressionKey, #impressions), that also implements the required
interface to enable sorting as well as conversion methods.
*/

type impressionCardinalityPair struct {
	key   string
	value int64
}

type impressionCardinalityPairList []impressionCardinalityPair

func (l impressionCardinalityPairList) Len() int           { return len(l) }
func (l impressionCardinalityPairList) Less(i, j int) bool { return l[i].value < l[j].value }
func (l impressionCardinalityPairList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l impressionCardinalityPairList) ToKeySlice() []string {
	res := make([]string, 0)
	for _, item := range l {
		res = append(res, item.key)
	}
	return res
}

func makeImpressionCardinalityPairList(data []interface{}) (*impressionCardinalityPairList, error) {
	output := make(impressionCardinalityPairList, 0)
	for i := 0; i < len(data); i += 2 {
		key, okKey := data[i].(string)
		if !okKey {
			return nil, fmt.Errorf("Error casting %v to string, it's %T", data[i], data[i])
		}
		value, okValue := data[i+1].(int64)
		if !okValue {
			return nil, fmt.Errorf("Error casting %v to int, it's %T", data[i+1], data[i+1])
		}
		output = append(output, impressionCardinalityPair{key, value})
	}
	return &output, nil
}

/* */

// ImpressionStorageAdapter implements ImpressionStorage interface
type ImpressionStorageAdapter struct {
	*BaseStorageAdapter
}

// NewImpressionStorageAdapter returns an instance of ImpressionStorageAdapter
func NewImpressionStorageAdapter(clientInstance redis.UniversalClient, prefix string) *ImpressionStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := ImpressionStorageAdapter{adapter}
	return &client
}

func (r ImpressionStorageAdapter) getImpressionsWithCardinality() (*impressionCardinalityPairList, error) {
	script := strings.Replace(
		impressionKeysWithCardinalityScriptTemplate, "{KEY_NAMESPACE}",
		r.impressionsNamespace("*", "*", "*"),
		1,
	)

	result, err := Client.Eval(script, nil, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("Failed to execute LUA script: %v", err.Error())
	}

	resultList, convOk := result.([]interface{})
	if !convOk {
		return nil, fmt.Errorf("Failed to type-assert script's output. %T", resultList)
	}

	impressionsWithCard, err := makeImpressionCardinalityPairList(resultList)
	if err != nil {
		return nil, fmt.Errorf("Failed to type-assert returned structure: %s", err.Error())
	}

	sort.Sort(impressionsWithCard)
	return impressionsWithCard, nil
}

func (r ImpressionStorageAdapter) getImpressionsWithoutCardinality() ([]string, error) {
	_keys, err := r.client.Keys(r.impressionsNamespace("*", "*", "*")).Result()
	if err == redis.Nil {
		log.Debug.Println("Fetching impression Keys", err.Error())
		return nil, err
	} else if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}
	return _keys, nil
}

func parseImpressionKey(key string) (string, string, string, error) {
	var re = regexp.MustCompile(`(\w+.)?SPLITIO\/([^\/]+)\/([^\/]+)\/impressions.([\s\S]*)`)
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

	featureName := match[4]
	if featureName == "" {
		return "", "", "", fmt.Errorf("Invalid feature name")
	}

	log.Verbose.Println("Impression parsed key", match)

	return sdkNameAndVersion, machineIP, featureName, nil
}

func parseRawImpressions(impressions []string) []api.ImpressionDTO {
	_keyImpressions := make([]api.ImpressionDTO, 0)
	for _, impression := range impressions {
		var impressionDTO api.ImpressionDTO
		err := json.Unmarshal([]byte(impression), &impressionDTO)
		if err != nil {
			log.Warning.Println("The impression cannot be decoded from JSON", err.Error())
			log.Verbose.Println("Impression value:", impression)
			continue
		}
		_keyImpressions = append(_keyImpressions, impressionDTO)
	}
	return _keyImpressions
}

func (r ImpressionStorageAdapter) removeImpressions(impressions []string, feature string) error {
	_impressionsToDelete := make([]interface{}, len(impressions))
	for i, v := range impressions {
		_impressionsToDelete[i] = v
	}

	beforeSRem := time.Now().UnixNano()
	if err := r.client.SRem(feature, _impressionsToDelete...).Err(); err != nil {
		return err
	}
	log.Benchmark.Println("Srem took", (time.Now().UnixNano() - beforeSRem))
	return nil
}

// RetrieveImpressions returns cached impressions
func (r ImpressionStorageAdapter) RetrieveImpressions() (map[string]map[string][]api.ImpressionsDTO, error) {

	var _keys []string
	// Attempt to fetch imressione keys using a LUA script to get both keys and number.
	// This will enable to sort features by # of impressions and use a greedy approach to
	// fetch as much impressions as possible.
	impressionsWithCard, err := r.getImpressionsWithCardinality()
	if err == nil {
		_keys = impressionsWithCard.ToKeySlice()
		// TODO: Use cardinality for reporting impression usage as well
	} else {
		// Something went wrong when trying to fetch using LUA script, fallback to simple method.
		_keys, err = r.getImpressionsWithoutCardinality()
		if err != nil {
			return nil, err
		}
	}

	impressionsToReturn := make(map[string]map[string][]api.ImpressionsDTO)

	log.Benchmark.Println("Impressions per post", conf.Data.ImpressionsPerPost)
	var impressionsPerKey = conf.Data.ImpressionsPerPost

	// At least one impression per feature
	if len(_keys) > 0 && impressionsPerKey >= int64(len(_keys)) {
		impressionsPerKey = int64(math.Max(float64(conf.Data.ImpressionsPerPost/int64(len(_keys))), float64(1)))
	}

	log.Benchmark.Println("Number of Keys", len(_keys))
	log.Benchmark.Println("Impressions per key", impressionsPerKey)

	// To optimize the impressions retrieval process, we track the extra slots from features
	// that have less impressions than `impressionsPerKey`. In order to use those slots
	// for other features.
	var extraQuota int64
	for _, feature := range _keys {
		log.Benchmark.Println("---", feature)
		beforeSRandMemberN := time.Now().UnixNano()
		impressions, err := r.client.SRandMemberN(feature, impressionsPerKey+extraQuota).Result()
		log.Benchmark.Println("SRandMemberN took", (time.Now().UnixNano() - beforeSRandMemberN))
		log.Verbose.Println(impressions)
		if err != nil {
			log.Error.Printf("Error fetching impressions from key %s. %s", feature, err.Error())
			continue
		}
		if len(impressions) == 0 {
			log.Debug.Println("No impressions found for feature ", feature)
			continue
		}

		extraQuota = (impressionsPerKey + extraQuota) - int64(len(impressions))

		_keyImpressions := parseRawImpressions(impressions)

		sdkNameAndVersion, machineIP, featureName, err := parseImpressionKey(feature)
		if err != nil {
			log.Error.Printf("Unable to parse key %s. Removing", feature)
			r.client.Del(feature)
			continue
		}

		if _, ok := impressionsToReturn[sdkNameAndVersion][machineIP]; !ok {
			impressionsToReturn[sdkNameAndVersion] = make(map[string][]api.ImpressionsDTO)
		}

		impressionsToReturn[sdkNameAndVersion][machineIP] = append(
			impressionsToReturn[sdkNameAndVersion][machineIP],
			api.ImpressionsDTO{TestName: featureName, KeyImpressions: _keyImpressions},
		)

		err = r.removeImpressions(impressions, feature)
		if err != nil {
			log.Error.Println("Error removing impressions from Redis", err.Error())
			log.Verbose.Println(impressions)
		}
	}
	return impressionsToReturn, nil
}
