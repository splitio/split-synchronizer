// Package api contains all functions and dtos Split APIs
package api

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

func before() {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	//Initialize by default
	conf.Initialize()

	conf.Data.Logger.DebugOn = true
}

func reset() {
	sdkClient = nil
	eventsClient = nil
}

func TestInitializeProd(t *testing.T) {
	before()
	os.Setenv(envSdkURLNamespace, "")
	os.Setenv(envEventsURLNamespace, "")

	Initialize()

	if sdkClient == nil {
		t.Error("SDK client not initialized")
	}

	if eventsClient == nil {
		t.Error("Events client not initialized")
	}

	reset()
}

func TestInitialize(t *testing.T) {
	before()

	os.Setenv(envSdkURLNamespace, "http://someurl.com")
	os.Setenv(envEventsURLNamespace, "http://someurl.com")

	Initialize()

	if sdkClient == nil {
		t.Error("SDK client not initialized")
	}

	if eventsClient == nil {
		t.Error("Events client not initialized")
	}

	reset()
}

func TestGet(t *testing.T) {
	before()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	txt, errg := sdkClient.Get("/")
	if errg != nil {
		t.Error(errg)
	}

	if string(txt) != "Hello, client\n" {
		t.Error("Given message failed ")
	}

	reset()
}

func TestGetGZIP(t *testing.T) {
	before()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Encoding", "gzip")

		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		fmt.Fprintln(gzw, "Hello, client")
	}))
	defer ts.Close()

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	txt, errg := sdkClient.Get("/")
	if errg != nil {
		t.Error(errg)
	}

	if string(txt) != "Hello, client\n" {
		t.Error("Given message failed ")
	}

	reset()
}

func TestPost(t *testing.T) {
	before()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	sdkClient.AddHeader("someHeader", "HeaderValue")
	errp := sdkClient.Post("/", []byte("some text"))
	if errp != nil {
		t.Error(errp)
	}

	reset()
}

func TestHeaders(t *testing.T) {
	before()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()

	os.Setenv(envSdkURLNamespace, ts.URL)
	os.Setenv(envEventsURLNamespace, ts.URL)

	Initialize()

	sdkClient.AddHeader("someHeader", "HeaderValue")
	_, ok1 := sdkClient.headers["someHeader"]
	if !ok1 {
		t.Error("Header could not be added")
	}

	sdkClient.ResetHeaders()
	_, ok2 := sdkClient.headers["someHeader"]
	if ok2 {
		t.Error("Reset Header fails")
	}

	reset()
}
