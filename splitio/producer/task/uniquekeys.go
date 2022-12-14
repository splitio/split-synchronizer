package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/provisional/strategy"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

// UniqueWorkerConfig bundles options
type UniqueWorkerConfig struct {
	Logger            logging.LoggerInterface
	Storage           storage.UniqueKeysMultiSdkConsumer
	UniqueKeysTracker strategy.UniqueKeysTracker
	URL               string
	Apikey            string
	FetchSize         int
	Metadata          dtos.Metadata
}

// UniqueKeysPipelineWorker implements all the required  methods to work with a pipelined task
type UniqueKeysPipelineWorker struct {
	logger            logging.LoggerInterface
	storage           storage.UniqueKeysMultiSdkConsumer
	uniqueKeysTracker strategy.UniqueKeysTracker

	url       string
	apikey    string
	fetchSize int64
	metadata  dtos.Metadata
}

func NewUniqueKeysWorker(cfg *UniqueWorkerConfig) Worker {
	return &UniqueKeysPipelineWorker{
		logger:            cfg.Logger,
		storage:           cfg.Storage,
		uniqueKeysTracker: cfg.UniqueKeysTracker,
		url:               cfg.URL + "/keys/ss",
		apikey:            cfg.Apikey,
		fetchSize:         int64(cfg.FetchSize),
		metadata:          cfg.Metadata,
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
	for _, raw := range raws {
		err, value := parseToObj(raw)
		if err != nil {
			err, value = parseToArray(raw)
			if err != nil {
				u.logger.Error("error deserializing fetched uniqueKeys: ", err.Error())
				continue
			}
		}

		for _, unique := range value {
			for _, key := range unique.Keys {
				u.uniqueKeysTracker.Track(unique.Feature, key)
			}
		}
	}

	uniques := u.uniqueKeysTracker.PopAll()
	if len(uniques.Keys) > 0 {
		sink <- uniques
	}

	return nil
}

func (u *UniqueKeysPipelineWorker) BuildRequest(data interface{}) (*http.Request, func(), error) {
	uniques, ok := data.(dtos.Uniques)
	if !ok {
		return nil, nil, fmt.Errorf("expected uniqueKeys. Got: %T", data)
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

func parseToArray(raw []byte) (error, []dtos.Key) {
	var queueObj []dtos.Key
	err := json.Unmarshal(raw, &queueObj)
	if err != nil {
		return err, nil
	}

	return nil, queueObj
}

func parseToObj(raw []byte) (error, []dtos.Key) {
	var queueObj dtos.Key
	err := json.Unmarshal(raw, &queueObj)
	if err != nil {
		return err, nil
	}

	return nil, []dtos.Key{queueObj}
}
