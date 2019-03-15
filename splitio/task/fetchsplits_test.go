// Package task contains all agent tasks
package task

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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

	var objmap map[string]*json.RawMessage
	if err := json.Unmarshal([]byte(mockedData), &objmap); err != nil {
		log.Error.Println(err)
		return nil, err
	}

	if err := json.Unmarshal(*objmap["splits"], &splitChangesDtoFromMock.RawSplits); err != nil {
		log.Error.Println(err)
		return nil, err
	}

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
func (h testSplitStorage) SplitsNames() ([]string, error)           { return nil, nil }
func (h testSplitStorage) RawSplits() ([]string, error)             { return nil, nil }

type testTrafficStorage struct{}

func (h testTrafficStorage) Decr(name string) error { return nil }
func (h testTrafficStorage) Incr(name string) error { return nil }
func TestFetchSplits(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	splitFetcherAdapterActive := testSplitFetcher{Status: "ACTIVE"}
	splitFetcherAdapterArchived := testSplitFetcher{Status: "ARCHIVED"}
	splitStorageAdapter := testSplitStorage{}
	trafficStorageAdapter := testTrafficStorage{}

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ACTIVE SPLIT
		taskFetchSplits(splitFetcherAdapterActive, splitStorageAdapter, trafficStorageAdapter)
	}()

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ARCHIVED SPLIT
		taskFetchSplits(splitFetcherAdapterArchived, splitStorageAdapter, trafficStorageAdapter)
	}()
}

func TestTrafficTypes(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	config := conf.NewInitializedConfigData()
	config.Redis.Prefix = "trafficTest"
	redis.Initialize(config.Redis)

	testKey := "trafficTest.SPLITIO.trafficType.user"

	redis.Client.Del(testKey)

	if redis.Client.Get(testKey).Val() != "" {
		t.Error("It should not exist")
	}

	redisStorageAdapter := redis.NewSplitStorageAdapter(redis.Client, "trafficTest")
	trafficStorageAdapter := redis.NewTrafficTypeStorageAdapter(redis.Client, "trafficTest")

	splitFetcherAdapterActive := testSplitFetcher{Status: "ACTIVE"}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ARCHIVED SPLIT
		taskFetchSplits(splitFetcherAdapterActive, redisStorageAdapter, trafficStorageAdapter)
	}()

	if redis.Client.Get(testKey).Val() != "1" {
		t.Error("It should be 1")
	}

	splitFetcherAdapterArchived := testSplitFetcher{Status: "ARCHIVED"}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ARCHIVED SPLIT
		taskFetchSplits(splitFetcherAdapterArchived, redisStorageAdapter, trafficStorageAdapter)
	}()

	if redis.Client.Get(testKey).Val() != "0" {
		t.Error("It should be 0")
	}
}

func TestTrafficTypesArchived(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	config := conf.NewInitializedConfigData()
	config.Redis.Prefix = "trafficTest"
	redis.Initialize(config.Redis)

	testKey := "trafficTest.SPLITIO.trafficType.user"

	redis.Client.Del(testKey)

	if redis.Client.Get(testKey).Val() != "" {
		t.Error("It should not exist")
	}

	redisStorageAdapter := redis.NewSplitStorageAdapter(redis.Client, "trafficTest")
	trafficStorageAdapter := redis.NewTrafficTypeStorageAdapter(redis.Client, "trafficTest")

	splitFetcherAdapterArchived := testSplitFetcher{Status: "ARCHIVED"}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		//Test ARCHIVED SPLIT
		taskFetchSplits(splitFetcherAdapterArchived, redisStorageAdapter, trafficStorageAdapter)
	}()

	if redis.Client.Get(testKey).Val() != "" {
		t.Error("It should not exist")
	}
}
