package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	cmnConf "github.com/splitio/go-split-commons/v6/conf"
	"github.com/splitio/go-split-commons/v6/dtos"
	cmnService "github.com/splitio/go-split-commons/v6/service"
	cmnAPI "github.com/splitio/go-split-commons/v6/service/api"
	"github.com/splitio/go-split-commons/v6/service/api/specs"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	// Unknown format
	Unknown = iota
	// Csv format
	Csv
)

type LargeSegmentFetcher interface {
	Fetch(name string, fetchOptions *cmnService.SegmentRequestParams) *dtos.LargeSegmentResponse
}

type HTTPLargeSegmentFetcher struct {
	client     cmnAPI.Client
	logger     logging.LoggerInterface
	memVersion *string
	httpClient *http.Client
}

// NewHTTPLargeSegmentsFetcher
func NewHTTPLargeSegmentFetcher(apikey string, memVersion string, cfg cmnConf.AdvancedConfig, logger logging.LoggerInterface, metadata dtos.Metadata) *HTTPLargeSegmentFetcher {
	return &HTTPLargeSegmentFetcher{
		client:     cmnAPI.NewHTTPClient(apikey, cfg, cfg.SdkURL, logger, metadata),
		logger:     logger,
		memVersion: &memVersion, // TODO (sanzmauro): move version to cmnConf.AdvancedConfig
		httpClient: &http.Client{},
	}
}

func (f *HTTPLargeSegmentFetcher) Fetch(name string, fetchOptions *cmnService.SegmentRequestParams) *dtos.LargeSegmentResponse {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/largeSegmentDefinition/")
	bufferQuery.WriteString(name)

	data, err := f.client.Get(bufferQuery.String(), fetchOptions)
	if err != nil {
		return &dtos.LargeSegmentResponse{
			Error: err,
			Retry: true,
		}
	}

	var rfeDTO dtos.RfeDTO
	err = json.Unmarshal(data, &rfeDTO)
	if err != nil {
		return &dtos.LargeSegmentResponse{
			Error: fmt.Errorf("error getting Request for Export: %s. %w", name, err),
			Retry: true,
		}
	}

	if time.Now().UnixMilli() > rfeDTO.ExpiresAt {
		return &dtos.LargeSegmentResponse{
			Error: fmt.Errorf("URL expired"),
			Retry: true,
		}
	}

	var toReturn dtos.LargeSegmentDTO
	retry, err := f.downloadAndParse(rfeDTO, &toReturn)
	if err != nil {
		return &dtos.LargeSegmentResponse{
			Error: err,
			Retry: retry,
		}
	}

	return &dtos.LargeSegmentResponse{
		Data:  &toReturn,
		Error: nil,
	}
}

func (f *HTTPLargeSegmentFetcher) downloadAndParse(rfe dtos.RfeDTO, tr *dtos.LargeSegmentDTO) (bool, error) {
	method := rfe.Params.Method
	if len(method) == 0 {
		method = http.MethodGet
	}

	req, _ := http.NewRequest(method, rfe.Params.URL, bytes.NewBuffer(rfe.Params.Body))
	req.Header = rfe.Params.Headers
	response, err := f.httpClient.Do(req)
	if err != nil {
		return true, err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return true,
			dtos.HTTPError{
				Code:    response.StatusCode,
				Message: response.Status,
			}
	}
	defer response.Body.Close()

	switch rfe.Format {
	case Csv:
		return csvReader(response, rfe, tr)
	default:
		return false, fmt.Errorf("unsupported file format")
	}
}

func csvReader(response *http.Response, rfe dtos.RfeDTO, tr *dtos.LargeSegmentDTO) (bool, error) {
	switch rfe.Version {
	case specs.MEMBERSHIP_V10:
		keys := make([]string, 0, rfe.TotalKeys)
		reader := csv.NewReader(response.Body)
		for {
			record, err := reader.Read()
			if err != nil {
				if err == io.EOF {
					break
				}

				return false, fmt.Errorf("error reading csv file. %w", err)
			}

			if l := len(record); l != 1 {
				return false, fmt.Errorf("unssuported file content. The file has multiple columns")
			}

			keys = append(keys, record[0])
		}

		tr.ChangeNumber = rfe.ChangeNumber
		tr.Name = rfe.Name
		tr.Keys = keys
		return false, nil
	default:
		return false, fmt.Errorf("unsupported csv version %s", rfe.Version)
	}
}

var _ LargeSegmentFetcher = (*HTTPLargeSegmentFetcher)(nil)
