// Package task contains all agent tasks
package task

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/storageDTOs"
)

var segmentMock1 = `
{
  "name": "employees",
  "added": [
    "user_for_testing_do_no_erase"
  ],
  "removed": [],
  "since": -1,
  "till": 1489542661161
}`

/* SegmentFetcher for testing */
type testSegmentFetcher struct {
	mockedPayload string
}

func (s testSegmentFetcher) Fetch(name string, changeNumber int64) (*api.SegmentChangesDTO, error) {

	var segmentChangesDto api.SegmentChangesDTO
	err := json.Unmarshal([]byte(s.mockedPayload), &segmentChangesDto)
	if err != nil {
		fmt.Println("Error parsing segment changes JSON for segment ", name, err)
		return nil, err
	}
	return &segmentChangesDto, nil
}

type testSegmentFetcherFactory struct {
	mockedPayload string
}

func (s testSegmentFetcherFactory) NewInstance() fetcher.SegmentFetcher {
	return testSegmentFetcher{
		mockedPayload: s.mockedPayload,
	}
}

/* SegmentStorage for testing */
type testSegmentStorage struct{}

func (s testSegmentStorage) RegisteredSegmentNames() ([]string, error) {
	return []string{"employees"}, nil
}
func (s testSegmentStorage) AddToSegment(segmentName string, keys []string) error         { return nil }
func (s testSegmentStorage) RemoveFromSegment(segmentName string, keys []string) error    { return nil }
func (s testSegmentStorage) SetChangeNumber(segmentName string, changeNumber int64) error { return nil }
func (s testSegmentStorage) ChangeNumber(segmentName string) (int64, error)               { return -1, nil }
func (s testSegmentStorage) CountActiveKeys(segmentName string) (int64, error)            { return 0, nil }
func (s testSegmentStorage) CountRemovedKeys(segmentName string) (int64, error)           { return 0, nil }
func (s testSegmentStorage) Keys(segmentName string) ([]storageDTOs.SegmentKeyDTO, error) {
	return nil, nil
}

type testSegmentStorageFactory struct{}

// NewInstance returns an instance of implemented SegmentStorage interface
func (s testSegmentStorageFactory) NewInstance() storage.SegmentStorage { return testSegmentStorage{} }

func TestTaskFetchSegments(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	//Initialize fetch segment task
	blocker = make(chan bool, 10)

	jobs = make(chan job)
	var jobsPool = make(map[string]*job)

	// worker to fetch jobs and run it.
	go worker()

	segmentFetcherAdapter := testSegmentFetcherFactory{mockedPayload: segmentMock1}
	storageAdapterFactory := testSegmentStorageFactory{}

	segmentsNames, _ := storageAdapterFactory.NewInstance().RegisteredSegmentNames()

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskFetchSegments(jobsPool, segmentsNames, segmentFetcherAdapter, storageAdapterFactory)
	}()

}

func TestTaskFetchSegmentsSaveTillSegment(t *testing.T) {
	var segmentMock2 = `
	{
		"name": "without_users",
	  	"added": [],
	  	"removed": [],
	  	"since": -1,
	  	"till": -1
	}`

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	//config := conf.NewInitializedConfigData()
	conf.Initialize()
	conf.Data.Redis.Prefix = "taskFetchSegment"
	redis.Initialize(conf.Data.Redis)

	//Initialize fetch segment task
	blocker = make(chan bool, 10)

	jobs = make(chan job)
	var jobsPool = make(map[string]*job)

	// worker to fetch jobs and run it.
	go worker()

	segmentsNames := []string{"without_users"}
	segmentFetcherAdapter := testSegmentFetcherFactory{mockedPayload: segmentMock2}
	storageAdapterFactory := redis.SegmentStorageMainFactory{}

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskFetchSegments(jobsPool, segmentsNames, segmentFetcherAdapter, storageAdapterFactory)
	}()

	time.Sleep(1 * time.Second)

	testKey := "taskFetchSegment.SPLITIO.segment.without_users.till"

	if redis.Client.Get(testKey).Val() != "-1" {
		t.Error("It should be -1")
	}

	redis.Client.Del(testKey)
}
