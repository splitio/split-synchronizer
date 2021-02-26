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

	"github.com/splitio/go-split-commons/v3/conf"
	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-split-commons/v3/service/api"
	"github.com/splitio/go-split-commons/v3/storage/mocks"
	"github.com/splitio/go-toolkit/v4/logging"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
)

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTaskCheckEnvirontmentStatusWithSomeFail(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}

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

	failClient := api.NewHTTPClient("fail", conf.GetDefaultAdvancedConfig(), fail.URL, log.Instance, dtos.Metadata{})
	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), ts.URL, log.Instance, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	CheckProducerStatus(mockStorage, common.HTTPClients{EventsClient: failClient, SdkClient: okClient, AuthClient: okClient})
	if !healthySince.IsZero() {
		t.Error("It should not write healthySince")
	}
}

func TestTaskCheckEnvirontmentStatus(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}

	tsHealthcheck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer tsHealthcheck.Close()

	os.Setenv("SPLITIO_SDK_URL", tsHealthcheck.URL)
	os.Setenv("SPLITIO_EVENTS_URL", tsHealthcheck.URL)

	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), tsHealthcheck.URL, log.Instance, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	check := time.Now()
	CheckProducerStatus(mockStorage, common.HTTPClients{EventsClient: okClient, SdkClient: okClient, AuthClient: okClient})
	if check.After(healthySince) {
		t.Error("It should succeed")
	}
}

func TestTaskCheckEnvirontmentStatusWithSomeFailAndSince(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}

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

	failClient := api.NewHTTPClient("fail", conf.GetDefaultAdvancedConfig(), fail.URL, log.Instance, dtos.Metadata{})
	okClient := api.NewHTTPClient("ok", conf.GetDefaultAdvancedConfig(), ts.URL, log.Instance, dtos.Metadata{})

	mockStorage := mocks.MockSplitStorage{
		ChangeNumberCall: func() (int64, error) { return 0, nil },
	}

	CheckProducerStatus(mockStorage, common.HTTPClients{EventsClient: failClient, SdkClient: okClient, AuthClient: okClient})
	if !healthySince.IsZero() {
		t.Error("It should be zero")
	}
}
