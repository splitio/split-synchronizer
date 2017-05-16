// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

var splitsMock = `{
  "splits": [%s],
  "since": -1,
  "till": 1491244291288
}`

var splitMock = `{
  "trafficTypeName": "user",
  "name": "SOME_SPLIT_TEST",
  "trafficAllocation": 100,
  "trafficAllocationSeed": 1314112417,
  "seed": -2059033614,
  "status": "ACTIVE",
  "killed": false,
  "defaultTreatment": "off",
  "changeNumber": 1491244291288,
  "algo": 2,
  "conditions": [
    {
      "conditionType": "ROLLOUT",
      "matcherGroup": {
        "combiner": "AND",
        "matchers": [
          {
            "keySelector": {
              "trafficType": "user",
              "attribute": null
            },
            "matcherType": "ALL_KEYS",
            "negate": false,
            "userDefinedSegmentMatcherData": null,
            "whitelistMatcherData": null,
            "unaryNumericMatcherData": null,
            "betweenMatcherData": null
          }
        ]
      },
      "partitions": [
        {
          "treatment": "on",
          "size": 0
        },
        {
          "treatment": "of",
          "size": 100
        }
      ],
      "label": "in segment all"
    }
  ]
}`

func TestHTTPSplitFetcher(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockedData := fmt.Sprintf(splitsMock, splitMock)
		fmt.Fprint(w, mockedData)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	httpSplitFetcher := NewHTTPSplitFetcher()
	splitFetched, err := httpSplitFetcher.Fetch(-1)
	if err != nil {
		t.Error(err)
	}

	if splitFetched.Since != -1 ||
		splitFetched.Till != 1491244291288 ||
		splitFetched.Splits[0].Name != "SOME_SPLIT_TEST" ||
		splitFetched.Splits[0].DefaultTreatment != "off" {
		t.Error("Fetched Split mal-formed")
	}
}

func TestHTTPSplitFetcherWithError(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	httpSplitFetcher := NewHTTPSplitFetcher()
	_, err := httpSplitFetcher.Fetch(-1)
	if err == nil {
		t.Error(err)
	}
}
