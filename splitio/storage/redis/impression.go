// Package redis implements different kind of storages for split information
package redis

import (
	"encoding/json"
	"regexp"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
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
func (r ImpressionStorageAdapter) RetrieveImpressions() ([]api.ImpressionsDTO, error) {

	_keys, err := r.client.Keys(r.impressionsNamespace("*", "*", "*")).Result()
	if err == redis.Nil {
		log.Debug.Println("Fetching impression Keys", err.Error())
		return nil, nil
	} else if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	var impressionsToReturn []api.ImpressionsDTO
	for _, key := range _keys {
		// TODO change by random impressions
		impressions, err := r.client.SMembers(key).Result()
		log.Debug.Println(impressions)
		if err != nil {
			log.Debug.Println("Fetching impressions", err.Error())
			continue
		}

		var _keyImpressions []api.ImpressionDTO
		//_keyImpressions := make([]api.ImpressionDTO, len(impressions))
		for _, impression := range impressions {
			var impressionDTO api.ImpressionDTO
			//impressionDTO = api.ImpressionDTO{}
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
		featureName := match[4]
		impressionsToReturn = append(impressionsToReturn, api.ImpressionsDTO{TestName: featureName, KeyImpressions: _keyImpressions})

	}

	return impressionsToReturn, nil
}
