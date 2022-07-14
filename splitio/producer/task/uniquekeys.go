package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

// UniqueWorkerConfig bundles options
type UniqueWorkerConfig struct {
	Logger    logging.LoggerInterface
	Storage   storage.EventMultiSdkConsumer
	URL       string
	Apikey    string
	FetchSize int
	Metadata  dtos.Metadata
}

// UniqueKeysPipelineWorker implements all the required  methods to work with a pipelined task
type UniqueKeysPipelineWorker struct {
	logger  logging.LoggerInterface
	storage storage.UniqueKeysMultiSdkConsumer

	url       string
	apikey    string
	fetchSize int64
	metadata  dtos.Metadata
}

func NewUniqueKeysWorker(cfg UniqueWorkerConfig) Worker {
	return &UniqueKeysPipelineWorker{
		logger:    cfg.Logger,
		storage:   cfg.Storage,
		url:       cfg.URL,
		apikey:    cfg.Apikey,
		fetchSize: int64(cfg.FetchSize),
		metadata:  cfg.Metadata,
	}
}

func (u *UniqueKeysPipelineWorker) Fetch() ([]string, error) {
	raw, _, err := u.storage.PopNRaw(u.fetchSize)
	if err != nil {
		return nil, fmt.Errorf("error fetching raw unique keys: %w", err)
	}

	return raw, nil
}
func (u *UniqueKeysPipelineWorker) Process(raws [][]byte, sink chan<- interface{}) error {
	filter := make(map[string]map[string]bool)
	for _, raw := range raws {
		var queueObj dtos.Uniques
		err := json.Unmarshal(raw, &queueObj)
		if err != nil {
			u.logger.Error("error deserializing fetched uniqueKeys: ", err.Error())
			continue
		}

		addUniqueToFilter(queueObj, filter)
	}

	sink <- buildUniquesObj(filter)

	return nil
}

func (u *UniqueKeysPipelineWorker) BuildRequest(data interface{}) (*http.Request, func(), error) {
	uniques, ok := data.(dtos.Uniques)
	if !ok {
		return nil, nil, fmt.Errorf("expected `uniqueKeys`. Got: %T", data)
	}

	serialized, err := json.Marshal(uniques)
	req, err := http.NewRequest("POST", u.url, bytes.NewReader(serialized))
	if err != nil {
		return nil, nil, fmt.Errorf("error building unique keys post request: %w", err)
	}

	req.Header = http.Header{}
	req.Header.Add("Authorization", "Bearer "+u.apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("SplitSDKVersion", u.metadata.SDKVersion)
	req.Header.Add("SplitSDKMachineIp", u.metadata.MachineIP)
	req.Header.Add("SplitSDKMachineName", u.metadata.MachineName)
	return req, nil, nil
}

func addUniqueToFilter(toAdd dtos.Uniques, filter map[string]map[string]bool) {
	for _, key := range toAdd.Keys {
		for _, ks := range key.Keys {
			if filter[key.Feature] != nil && filter[key.Feature][ks] {
				continue
			}

			if filter[key.Feature] == nil {
				filter[key.Feature] = make(map[string]bool)
			}

			filter[key.Feature][ks] = true
		}
	}
}

func buildUniquesObj(filter map[string]map[string]bool) dtos.Uniques {
	toReturn := dtos.Uniques{Keys: []dtos.Key{}}

	for k := range filter {
		keys := make([]string, 0, len(filter[k]))
		for ks := range filter[k] {
			keys = append(keys, ks)
		}

		toAdd := dtos.Key{
			Feature: k,
			Keys:    keys,
		}

		toReturn.Keys = append(toReturn.Keys, toAdd)
	}

	return toReturn
}
