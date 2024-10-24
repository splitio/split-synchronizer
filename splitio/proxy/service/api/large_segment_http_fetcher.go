package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	cmnConf "github.com/splitio/go-split-commons/v6/conf"
	cmnDTOs "github.com/splitio/go-split-commons/v6/dtos"
	cmnService "github.com/splitio/go-split-commons/v6/service"
	cmnAPI "github.com/splitio/go-split-commons/v6/service/api"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/service/dtos"
)

var MEM_VERSION_10 = "1.0"

const (
	// Unknown format
	Unknown = iota
	// Csv format
	Csv
)

type LargeSegmentFetcher interface {
	Fetch(name string, fetchOptions *cmnService.SegmentRequestParams) (*dtos.LargeSegmentDTO, error)
}

type HTTPLargeSegmentFetcher struct {
	client     cmnAPI.Client
	logger     logging.LoggerInterface
	memVersion *string
	httpClient *http.Client
}

// NewHTTPLargeSegmentsFetcher
func NewHTTPLargeSegmentFetcher(apikey string, cfg cmnConf.AdvancedConfig, logger logging.LoggerInterface, metadata cmnDTOs.Metadata) LargeSegmentFetcher {
	return &HTTPLargeSegmentFetcher{
		client:     cmnAPI.NewHTTPClient(apikey, cfg, cfg.SdkURL, logger, metadata),
		logger:     logger,
		memVersion: &MEM_VERSION_10,
		httpClient: &http.Client{},
	}
}

func (f *HTTPLargeSegmentFetcher) Fetch(name string, fetchOptions *cmnService.SegmentRequestParams) (*dtos.LargeSegmentDTO, error) {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/proxy/largeSegment/")
	bufferQuery.WriteString(name)

	data, err := f.client.Get(bufferQuery.String(), fetchOptions)
	if err != nil {
		f.logger.Error(err.Error())
		return nil, err
	}

	var rfeDTO dtos.RfeDTO
	err = json.Unmarshal(data, &rfeDTO)
	if err != nil {
		f.logger.Error("Error getting Request for Export: ", name, err)
		return nil, err
	}

	keys, err := f.downloadAndParse(rfeDTO)
	if err != nil {
		return nil, err
	}

	return &dtos.LargeSegmentDTO{
		Name: name,
		Keys: keys,
	}, nil
}

func (f *HTTPLargeSegmentFetcher) downloadAndParse(rfe dtos.RfeDTO) ([]string, error) {
	method := rfe.Params.Method
	if len(method) == 0 {
		method = http.MethodGet
	}

	req, _ := http.NewRequest(method, rfe.Params.URL, nil)
	req.Header = rfe.Params.Headers
	response, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	fmt.Println(response.StatusCode)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, cmnDTOs.HTTPError{
			Code:    response.StatusCode,
			Message: response.Status,
		}
	}
	defer response.Body.Close()

	switch rfe.Format {
	case Csv:
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}

		return csv(rfe.Version, body)
	default:
		return nil, fmt.Errorf("unsupported file format")
	}
}

func csv(version string, body []byte) ([]string, error) {
	switch version {
	case MEM_VERSION_10:
		return strings.Split(string(body), "\n"), nil
	default:
		return nil, fmt.Errorf("unsupported csv version %s", version)
	}
}
