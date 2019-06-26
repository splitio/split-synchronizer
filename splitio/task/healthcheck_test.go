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

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type mockStorage struct {
	shouldFail bool
}

var succeed time.Time

func TestTaskCheckEnvirontmentStatusWithSomeFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

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

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
	})

	splitStorageAdapter := testSplitStorage{}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskCheckEnvirontmentStatus(splitStorageAdapter)
		if !lastSucceed.IsZero() {
			t.Error("It should not write lastSucceed")
		}
	}()
}

func TestTaskCheckEnvirontmentStatus(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	tsHealthcheck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer tsHealthcheck.Close()

	os.Setenv("SPLITIO_SDK_URL", tsHealthcheck.URL)
	os.Setenv("SPLITIO_EVENTS_URL", tsHealthcheck.URL)

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
	})

	splitStorageAdapter := testSplitStorage{}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		check := time.Now()
		taskCheckEnvirontmentStatus(splitStorageAdapter)
		if check.After(lastSucceed) {
			t.Error("It should succeed")
		}
		succeed = lastSucceed
	}()
}

func TestTaskCheckEnvirontmentStatusWithSomeFailAndLastSucceed(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

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

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
	})

	splitStorageAdapter := testSplitStorage{}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskCheckEnvirontmentStatus(splitStorageAdapter)
		if lastSucceed != succeed {
			t.Error("It should not write new lastSucceed")
		}
	}()
}
