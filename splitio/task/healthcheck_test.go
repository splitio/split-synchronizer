// Package task contains all agent tasks
package task

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage/mocks"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/log"
)

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTaskCheckEnvirontmentStatusWithSomeFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	logger := logging.NewLogger(&logging.LoggerOptions{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer fail.Close()

	os.Setenv("SPLITIO_SDK_URL", fail.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	failClient := api.NewHTTPClient("fail", conf.GetDefaultAdvancedConfig(), fail.URL, logger, dtos.Metadata{})
	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), ts.URL, logger, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	CheckProducerStatus(mockStorage, okClient, failClient)
	if !healthySince.IsZero() {
		t.Error("It should not write healthySince")
	}
}

func TestTaskCheckEnvirontmentStatus(t *testing.T) {
	stdoutWriter := ioutil.Discard // os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	logger := logging.NewLogger(&logging.LoggerOptions{})

	tsHealthcheck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer tsHealthcheck.Close()

	os.Setenv("SPLITIO_SDK_URL", tsHealthcheck.URL)
	os.Setenv("SPLITIO_EVENTS_URL", tsHealthcheck.URL)

	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), tsHealthcheck.URL, logger, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	check := time.Now()
	CheckProducerStatus(mockStorage, okClient, okClient)
	if check.After(healthySince) {
		t.Error("It should succeed")
	}
}

func TestTaskCheckEnvirontmentStatusWithSomeFailAndSince(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	logger := logging.NewLogger(&logging.LoggerOptions{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer fail.Close()

	os.Setenv("SPLITIO_SDK_URL", fail.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	failClient := api.NewHTTPClient("fail", conf.GetDefaultAdvancedConfig(), fail.URL, logger, dtos.Metadata{})
	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), ts.URL, logger, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	CheckProducerStatus(mockStorage, failClient, okClient)
	if !healthySince.IsZero() {
		t.Error("It should be zero")
	}
}
