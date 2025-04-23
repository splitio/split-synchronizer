package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-split-commons/v6/storage"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

// UniqueWorkerConfig bundles options
type UniqueWorkerConfig struct {
	Logger    logging.LoggerInterface
	Storage   storage.UniqueKeysMultiSdkConsumer
	URL       string
	Apikey    string
	FetchSize int
	Metadata  dtos.Metadata
}

// UniqueKeysPipelineWorker implements all the required  methods to work with a pipelined task
type UniqueKeysPipelineWorker struct {
	logger    logging.LoggerInterface
	storage   storage.UniqueKeysMultiSdkConsumer
	url       string
	apikey    string
	fetchSize int
	metadata  dtos.Metadata
}

func NewUniqueKeysWorker(cfg *UniqueWorkerConfig) Worker {
	return &UniqueKeysPipelineWorker{
		logger:    cfg.Logger,
		storage:   cfg.Storage,
		url:       cfg.URL + "/keys/ss",
		apikey:    cfg.Apikey,
		fetchSize: cfg.FetchSize,
		metadata:  cfg.Metadata,
	}
}

func (u *UniqueKeysPipelineWorker) Fetch() ([]string, error) {
	raw, _, err := u.storage.PopNRaw(int64(u.fetchSize))
	if err != nil {
		return nil, fmt.Errorf("error fetching raw unique keys: %w", err)
	}

	return raw, nil
}

func (u *UniqueKeysPipelineWorker) Process(raws [][]byte, sink chan<- interface{}) error {
	rawKeys := make([]dtos.Key, 0)
	for _, raw := range raws {
		value, err := parseToObj(raw)
		if err == nil {
			u.logger.Debug("Unique Keys parsed to Dto.")
		}

		if err != nil {
			value, err = parseToArray(raw)
			if err != nil {
				u.logger.Error("error deserializing fetched uniqueKeys: ", err.Error())
				continue
			}
			u.logger.Debug("Unique Keys parsed to Array.")
		}

		rawKeys = append(rawKeys, value...)
	}

	filtered := cleanUp(rawKeys)
	groups := batches(filtered, u.fetchSize)

	for index := range groups {
		sink <- groups[index]
	}

	return nil
}

func (u *UniqueKeysPipelineWorker) BuildRequest(data interface{}) (*http.Request, error) {
	uniques, ok := data.(dtos.Uniques)
	if !ok {
		return nil, fmt.Errorf("expected uniqueKeys. Got: %T", data)
	}

	serialized, err := json.Marshal(uniques)
	req, err := http.NewRequest("POST", u.url, bytes.NewReader(serialized))
	if err != nil {
		return nil, fmt.Errorf("error building unique keys post request: %w", err)
	}

	req.Header = http.Header{}
	req.Header.Add("Authorization", "Bearer "+u.apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("SplitSDKVersion", u.metadata.SDKVersion)
	req.Header.Add("SplitSDKMachineIp", u.metadata.MachineIP)
	req.Header.Add("SplitSDKMachineName", u.metadata.MachineName)
	return req, nil
}

func parseToArray(raw []byte) ([]dtos.Key, error) {
	var queueObj []dtos.Key
	err := json.Unmarshal(raw, &queueObj)
	if err != nil {
		return nil, err
	}

	return queueObj, nil
}

func parseToObj(raw []byte) ([]dtos.Key, error) {
	var queueObj dtos.Key
	err := json.Unmarshal(raw, &queueObj)
	if err != nil {
		return nil, err
	}

	return []dtos.Key{queueObj}, nil
}

func cleanUp(keys []dtos.Key) map[string]*set.ThreadUnsafeSet {
	filtered := make(map[string]*set.ThreadUnsafeSet)
	for _, key := range keys {
		_, exists := filtered[key.Feature]
		if !exists {
			filtered[key.Feature] = set.NewSet()
		}
		filtered[key.Feature].Add(key.Keys...)
	}

	return filtered
}

func batches(filtered map[string]*set.ThreadUnsafeSet, maxSize int) []dtos.Uniques {
	groups := make([]dtos.Uniques, 0)
	currentBatch := dtos.Uniques{Keys: []dtos.Key{}}
	currentBatchSize := 0

	for name, keys := range filtered {
		keyList := keys.List()
		totalKeys := len(keyList)
		start := 0

		for start < totalKeys {
			end := start + maxSize - currentBatchSize
			if end > totalKeys {
				end = totalKeys
			}

			// Add keys to the current batch
			keyDto := dtos.Key{
				Feature: name,
				Keys:    keyList[start:end],
			}
			currentBatch.Keys = append(currentBatch.Keys, keyDto)
			currentBatchSize += len(keyList[start:end])

			// If the current batch reaches maxSize, finalize it and start a new one
			if currentBatchSize >= maxSize {
				groups = append(groups, currentBatch)
				currentBatch = dtos.Uniques{Keys: []dtos.Key{}}
				currentBatchSize = 0
			}

			start = end
		}
	}

	// Add the remaining batch if it has any keys
	if currentBatchSize > 0 {
		groups = append(groups, currentBatch)
	}

	return groups
}
