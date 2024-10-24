package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"os"

	cmnConf "github.com/splitio/go-split-commons/v6/conf"
	cmnDTOs "github.com/splitio/go-split-commons/v6/dtos"
	cmnService "github.com/splitio/go-split-commons/v6/service"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/service/dtos"
	"github.com/stretchr/testify/assert"
)

func TestFetchCsvFormat(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	test_csv, _ := os.ReadFile("testdata/large_segment_test.csv")
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(test_csv)
	}))
	defer fileServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "GET",
				URL:    fileServer.URL,
			},
			Format:       Csv,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1.0",
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		cmnConf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
		cmnDTOs.Metadata{},
	)

	lsData, err := fetcher.Fetch("large_segment_test", &cmnService.SegmentRequestParams{})
	if err != nil {
		t.Error(err)
	}

	assert.Equal(t, "large_segment_test", lsData.Name)
	assert.Equal(t, 1500, len(lsData.Keys))
}

func TestFetchCsvFormatWithOtherVersion(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	test_csv, _ := os.ReadFile("testdata/large_segment_test.csv")
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(test_csv)
	}))
	defer fileServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "GET",
				URL:    fileServer.URL,
			},
			Format:       Csv,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1111.0",
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		cmnConf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
		cmnDTOs.Metadata{},
	)

	lsData, err := fetcher.Fetch("large_segment_test", &cmnService.SegmentRequestParams{})

	assert.Equal(t, "unsupported csv version 1111.0", err.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), lsData)
}

func TestFetchUnknownFormat(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	test_csv, _ := os.ReadFile("testdata/large_segment_test.csv")
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(test_csv)
	}))
	defer fileServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "GET",
				URL:    fileServer.URL,
			},
			Format:       Unknown,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1.0",
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		cmnConf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
		cmnDTOs.Metadata{},
	)

	lsData, err := fetcher.Fetch("large_segment_test", &cmnService.SegmentRequestParams{})

	assert.Equal(t, "unsupported file format", err.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), lsData)
}

func TestFetchAPIError(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		cmnConf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
		cmnDTOs.Metadata{},
	)

	lsData, err := fetcher.Fetch("large_segment_test", &cmnService.SegmentRequestParams{})
	assert.Equal(t, "500 Internal Server Error", err.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), lsData)
}

func TestFetchDownloadServerError(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))
	defer fileServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "GET",
				URL:    fileServer.URL,
			},
			Format:       Csv,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1.0",
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		cmnConf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
		cmnDTOs.Metadata{},
	)

	lsData, err := fetcher.Fetch("large_segment_test", &cmnService.SegmentRequestParams{})
	assert.Equal(t, "500 Internal Server Error", err.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), lsData)
}
