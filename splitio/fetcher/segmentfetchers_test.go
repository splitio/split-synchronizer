// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

var segmentMock = `
{
  "name": "employees",
  "added": [
    "user_for_testing_do_no_erase"
  ],
  "removed": [],
  "since": -1,
  "till": 1489542661161
}`

func TestHTTPSegmentFetcher(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, segmentMock)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	segmentFetcherFactory := SegmentFetcherMainFactory{}
	segmentFetcher := segmentFetcherFactory.NewInstance()

	segmentFetched, err := segmentFetcher.Fetch("employees", -1)
	if err != nil {
		t.Error(err)
		return
	}

	if segmentFetched.Name != "employees" ||
		segmentFetched.Since != -1 ||
		segmentFetched.Till != 1489542661161 ||
		segmentFetched.Added[0] != "user_for_testing_do_no_erase" ||
		len(segmentFetched.Removed) != 0 {
		t.Error("Fetched segment mal-formed")
	}
}

func TestHTTPSegmentFetcherWithError(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	segmentFetcherFactory := SegmentFetcherMainFactory{}
	segmentFetcher := segmentFetcherFactory.NewInstance()

	_, err := segmentFetcher.Fetch("employees", -1)
	if err == nil {
		t.Error(err)
	}
}
