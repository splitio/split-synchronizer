package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"os"

	"github.com/splitio/go-split-commons/v6/conf"
	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-split-commons/v6/service"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

func TestFetchCsvFormatHappyPath(t *testing.T) {
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
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	if response.Error != nil {
		t.Error("Error should be nil")
		fmt.Println(response.Error)
	}

	assert.Equal(t, "large_segment_test", response.Data.Name)
	assert.Equal(t, 1500, len(response.Data.Keys))
}

func TestFetchCsvMultipleColumns(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	test_csv, _ := os.ReadFile("testdata/ls_wrong.csv")
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
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "unssuported file content. The file has multiple columns", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
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
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "unsupported csv version 1111.0", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
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
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "unsupported file format", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
}

func TestFetchAPIError(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "500 Internal Server Error", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
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
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "500 Internal Server Error", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
}

func TestFetchWithPost(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	test_csv, _ := os.ReadFile("testdata/large_segment_test.csv")
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(test_csv)
	}))
	defer fileServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "POST",
				URL:    fileServer.URL,
			},
			Format:       Csv,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1.0",
			ExpiresAt:    time.Now().UnixMilli() + 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	if response.Error != nil {
		t.Error("Error shuld be nil")
	}

	assert.Equal(t, "large_segment_test", response.Data.Name)
	assert.Equal(t, 1500, len(response.Data.Keys))
}

func TestFetcahURLExpired(t *testing.T) {
	logger := logging.NewLogger(&logging.LoggerOptions{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(dtos.RfeDTO{
			Params: dtos.ParamsDTO{
				Method: "GET",
				URL:    "http://localhost",
			},
			Format:       Csv,
			TotalKeys:    1500,
			Size:         100,
			ChangeNumber: 100,
			Name:         "large_segment_test",
			Version:      "1.0",
			ExpiresAt:    time.Now().UnixMilli() - 10000,
		})
		w.Write(data)
	}))
	defer ts.Close()

	fetcher := NewHTTPLargeSegmentFetcher(
		"api-key",
		"1.0",
		conf.AdvancedConfig{
			SdkURL: ts.URL,
		},
		logger,
		dtos.Metadata{},
	)

	response := fetcher.Fetch("large_segment_test", &service.SegmentRequestParams{})
	assert.Equal(t, "URL expired", response.Error.Error())
	assert.Equal(t, (*dtos.LargeSegmentDTO)(nil), response.Data)
}
