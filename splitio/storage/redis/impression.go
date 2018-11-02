package redis

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"fmt"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	redis "gopkg.in/redis.v5"
)

var impressionMutex sync.Mutex

const minImpressionsPerFeature = 50

// ImpressionObject obect
type ImpressionObject struct {
	KeyName           string `json:"k"`
	BucketingKey      string `json:"b"`
	FeatureName       string `json:"f"`
	Treatment         string `json:"t"`
	SplitChangeNumber int64  `json:"t"`
	Rule              string `json:"r"`
	Timestamp         int64  `json:"m"`
}

// ImpressionMetadata object
type ImpressionMetadata struct {
	SdkVersion   string `json:"s"`
	InstanceIP   string `json:"i"`
	InstanceName string `json:"n"`
}

func (m *ImpressionMetadata) toSdkMetadata() api.SdkMetadata {
	return api.SdkMetadata{
		MachineIP:   m.InstanceIP,
		MachineName: m.InstanceName,
		SdkVersion:  m.SdkVersion,
	}
}

// ImpressionDTO object
type ImpressionDTO struct {
	Data     ImpressionObject   `json:"i"`
	Metadata ImpressionMetadata `json:"m"`
}

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
func NewImpressionStorageAdapter(clientInstance *redis.Client, prefix string) *ImpressionStorageAdapter {
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

func (r ImpressionStorageAdapter) fetchImpressionsFromQueueWithLock(count int64) ([]string, error) {
	impressionMutex.Lock()
	defer impressionMutex.Unlock()

	lrangeResult := r.client.LRange(r.impressionsQueueNamespace(), 0, int64(count-1))
	if lrangeResult.Err() != nil {
		log.Error.Println("Fetching impressions", lrangeResult.Err().Error())
		return nil, lrangeResult.Err()
	}

	fetchedCount := int64(len(lrangeResult.Val()))
	lTrimResult := r.client.LTrim(r.impressionsQueueNamespace(), fetchedCount, int64(-1))
	if lTrimResult.Err() != nil {
		log.Error.Println("Trim events", lTrimResult.Err().Error())
		return nil, lTrimResult.Err()
	}

	return lrangeResult.Val(), nil
}

func toImpressionsDTO(impressionsMap map[string][]api.ImpressionDTO) ([]api.ImpressionsDTO, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]api.ImpressionsDTO, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, api.ImpressionsDTO{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

// FetchImpressionsFromQueue retrieves impression from a redis list acting as a queue.
func (r ImpressionStorageAdapter) fetchImpressionsFromQueue(count int64) (map[api.SdkMetadata][]api.ImpressionsDTO, error) {

	impressionsRawList, err := r.fetchImpressionsFromQueueWithLock(count)
	if err != nil {
		return nil, err
	}

	// grouping the information by instanceID/instanceIP, and then by feature name
	collectedData := make(map[ImpressionMetadata]map[string][]api.ImpressionDTO)

	for _, rawImpression := range impressionsRawList {
		var impression ImpressionDTO
		err := json.Unmarshal([]byte(rawImpression), &impression)
		if err != nil {
			log.Error.Println("Error decoding impression JSON", err.Error())
			continue
		}

		_, instanceExists := collectedData[impression.Metadata]
		if !instanceExists {
			collectedData[impression.Metadata] = make(map[string][]api.ImpressionDTO)
		}

		_, featureExists := collectedData[impression.Metadata][impression.Data.FeatureName]
		if !featureExists {
			collectedData[impression.Metadata][impression.Data.FeatureName] = make([]api.ImpressionDTO, 0)
		}

		collectedData[impression.Metadata][impression.Data.FeatureName] = append(
			collectedData[impression.Metadata][impression.Data.FeatureName],
			api.ImpressionDTO{
				BucketingKey: impression.Data.BucketingKey,
				ChangeNumber: impression.Data.SplitChangeNumber,
				KeyName:      impression.Data.KeyName,
				Label:        impression.Data.Rule,
				Time:         impression.Data.Timestamp,
				Treatment:    impression.Data.Treatment,
			},
		)
	}

	toReturn := make(map[api.SdkMetadata][]api.ImpressionsDTO)
	for metadata, impsForMetadata := range collectedData {
		toReturn[metadata.toSdkMetadata()], err = toImpressionsDTO(impsForMetadata)
		if err != nil {
			log.Error.Printf("Unable to write impressions for metadata %v", metadata)
			continue
		}
	}

	return toReturn, nil
}

// RetrieveImpressions returns cached impressions
func (r ImpressionStorageAdapter) fetchImpressionsLegacy(count int64) (map[api.SdkMetadata][]api.ImpressionsDTO, error) {

	var _keys []string
	log.Benchmark.Println("Impressions per post", count)

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

	impressionsToReturn := make(map[api.SdkMetadata][]api.ImpressionsDTO)
	if len(_keys) == 0 {
		return impressionsToReturn, nil
	}

	impressionsPerKey := int64(math.Max(float64(count/int64(len(_keys))), float64(minImpressionsPerFeature)))
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

		meta := api.SdkMetadata{
			MachineIP:   machineIP,
			SdkVersion:  sdkNameAndVersion,
			MachineName: "", // Unfortunately this redis scheme doesn't support machine name.
		}

		if _, ok := impressionsToReturn[meta]; !ok {
			impressionsToReturn[meta] = make([]api.ImpressionsDTO, 0)
		}

		impressionsToReturn[meta] = append(
			impressionsToReturn[meta],
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

func mergeImpressionsDTOSlices(slice1 []api.ImpressionsDTO, slice2 []api.ImpressionsDTO) ([]api.ImpressionsDTO, error) {

	// Edge cases
	if slice1 == nil && slice2 == nil {
		return nil, fmt.Errorf("Both slices cannot be nil")
	}

	if slice1 == nil {
		tmp := make([]api.ImpressionsDTO, len(slice2))
		copy(tmp, slice2)
		return tmp, nil
	}

	if slice2 == nil {
		tmp := make([]api.ImpressionsDTO, len(slice1))
		copy(tmp, slice1)
		return tmp, nil
	}

	// Main algorithm
	sort.Slice(slice1, func(i, j int) bool { return slice1[i].TestName < slice1[j].TestName })
	sort.Slice(slice2, func(i, j int) bool { return slice2[i].TestName < slice2[j].TestName })
	index1 := 0
	index2 := 0
	length1 := len(slice1)
	length2 := len(slice2)
	output := make([]api.ImpressionsDTO, 0)
	for index1 < length1 && index2 < length2 {
		if slice1[index1].TestName < slice2[index2].TestName {
			output = append(output, slice1[index1])
			index1++
		} else if slice2[index2].TestName < slice1[index1].TestName {
			output = append(output, slice2[index2])
			index2++
		} else {
			// The name is equal -> we need to merge
			merge := append(slice1[index1].KeyImpressions, slice2[index2].KeyImpressions...)
			output = append(output, api.ImpressionsDTO{
				KeyImpressions: merge,
				TestName:       slice1[index1].TestName,
			})
			index1++
			index2++
		}
	}

	// Carryover
	if index1 < length1 {
		for remainingIndex := index1; remainingIndex < length1; remainingIndex++ {
			output = append(output, slice1[remainingIndex])
		}
	} else if index2 < length2 {
		for remainingIndex := index2; remainingIndex < length2; remainingIndex++ {
			output = append(output, slice2[remainingIndex])
		}
	}

	return output, nil
}

// RetrieveImpressions returns impressions stored in redis
func (r ImpressionStorageAdapter) RetrieveImpressions(count int64, legacyEnabled bool) (map[api.SdkMetadata][]api.ImpressionsDTO, error) {
	impressions, err := r.fetchImpressionsFromQueue(count)
	if err != nil {
		return nil, err
	}

	if legacyEnabled {
		legacyImpressions, err := r.fetchImpressionsLegacy(count)
		if err != nil {
			log.Error.Println("Legacy impressions fetching is enabled, but failed to execute:")
			log.Error.Println(err.Error())
			return impressions, nil
		}
		for key := range legacyImpressions {
			if _, exists := impressions[key]; !exists {
				impressions[key] = legacyImpressions[key]
			} else {
				merged, err := mergeImpressionsDTOSlices(impressions[key], legacyImpressions[key])
				if err != nil {
					log.Error.Printf(
						"Queue and legacy impressions found for metadata %+v, but merging failed. Keeping only queued ones.",
						key,
					)
					continue
				}
				impressions[key] = merged
			}
		}
	}
	return impressions, nil
}
