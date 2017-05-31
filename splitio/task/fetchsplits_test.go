// Package task contains all agent tasks
package task

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/splitio/go-agent/conf"
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
  "name": "DEMO_MURMUR2",
  "trafficAllocation": 100,
  "trafficAllocationSeed": 1314112417,
  "seed": -2059033614,
  "status": "%s",
  "killed": false,
  "defaultTreatment": "of",
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
            "matcherType": "IN_SEGMENT",
            "negate": false,
            "userDefinedSegmentMatcherData": {
              "segmentName": "employees"
            },
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

/* SplitFetcher for testing */
type testSplitFetcher struct {
	Status string
}

func (h testSplitFetcher) Fetch(changeNumber int64) (*api.SplitChangesDTO, error) {
	var mockedData string
	if h.Status == "ACTIVE" {
		mockedData = fmt.Sprintf(splitsMock, fmt.Sprintf(splitMock, "ACTIVE"))
	} else {
		mockedData = fmt.Sprintf(splitsMock, fmt.Sprintf(splitMock, "ARCHIVED"))
	}

	var splitChangesDtoFromMock api.SplitChangesDTO

	err := json.Unmarshal([]byte(mockedData), &splitChangesDtoFromMock)
	if err != nil {
		fmt.Println("Error parsing split changes JSON ", err)
		return nil, err
	}

	return &splitChangesDtoFromMock, nil
}

/* SplitStorage for testing*/
type testSplitStorage struct{}

func (h testSplitStorage) Save(split interface{}) error             { return nil }
func (h testSplitStorage) Remove(split interface{}) error           { return nil }
func (h testSplitStorage) RegisterSegment(name string) error        { return nil }
func (h testSplitStorage) SetChangeNumber(changeNumber int64) error { return nil }
func (h testSplitStorage) ChangeNumber() (int64, error)             { return 1491244291288, nil }

func TestFetchSplits(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	splitFetcherAdapterActive := testSplitFetcher{Status: "ACTIVE"}
	splitFetcherAdapterArchived := testSplitFetcher{Status: "ARCHIVED"}
	splitStorageAdapter := testSplitStorage{}

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ACTIVE SPLIT
		taskFetchSplits(splitFetcherAdapterActive, splitStorageAdapter)
	}()

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ARCHIVED SPLIT
		taskFetchSplits(splitFetcherAdapterArchived, splitStorageAdapter)
	}()

}
