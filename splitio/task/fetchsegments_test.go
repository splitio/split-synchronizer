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
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/storage"
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

/* SegmentFetcher for testing */
type testSegmentFetcher struct{}

func (s testSegmentFetcher) Fetch(name string, changeNumber int64) (*api.SegmentChangesDTO, error) {

	var segmentChangesDto api.SegmentChangesDTO
	err := json.Unmarshal([]byte(segmentMock), &segmentChangesDto)
	if err != nil {
		fmt.Println("Error parsing segment changes JSON for segment ", name, err)
		return nil, err
	}
	return &segmentChangesDto, nil
}

type testSegmentFetcherFactory struct{}

func (s testSegmentFetcherFactory) NewInstance() fetcher.SegmentFetcher {
	return testSegmentFetcher{}
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

	segmentFetcherAdapter := testSegmentFetcherFactory{}
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
