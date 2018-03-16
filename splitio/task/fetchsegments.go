package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/stats/counter"
	"github.com/splitio/split-synchronizer/splitio/stats/latency"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

var segmentsIncoming = make(chan string, 1)

// StopFetchSegments stops FetchSplits task sendding signal
func StopFetchSegments() {
	select {
	case segmentsIncoming <- "STOP":
	default:
	}
}

var segmentChangeFetcherLatencies = latency.NewLatencyBucket()
var segmentChangeFetcherCounters = counter.NewCounter()
var segmentChangeFetcherLocalCounters = counter.NewLocalCounter()

var blocker chan bool
var jobs chan job
var jobsWaitingGroup sync.WaitGroup

type job struct {
	segmentName    string
	segmentFetcher fetcher.SegmentFetcher
	segmentStorage storage.SegmentStorage
}

func (j job) run() {
	// Decrement the counter when the goroutine completes.
	defer jobsWaitingGroup.Done()

	blocker <- true

	tryNumber := 3

	for tryNumber > 0 {

		lastChangeNumber, err := j.segmentStorage.ChangeNumber(j.segmentName)
		if err != nil {
			log.Debug.Printf("Fetching change number for segment %s: %s\n", j.segmentName, err.Error())
			lastChangeNumber = -1
		}

		startTime := segmentChangeFetcherLatencies.StartMeasuringLatency()
		segment, errSegmentFetch := j.segmentFetcher.Fetch(j.segmentName, lastChangeNumber)
		if errSegmentFetch != nil {

			if _, ok := errSegmentFetch.(*api.HttpError); ok {
				segmentChangeFetcherLocalCounters.Increment("backend::request.error")
				segmentChangeFetcherCounters.Increment(fmt.Sprintf("segmentChangeFetcher.status.%d", errSegmentFetch.(*api.HttpError).Code))
			}

			log.Error.Println("Error fetching segment ", j.segmentName, errSegmentFetch.Error())
			tryNumber--
			time.Sleep(time.Second * 5)
			continue
		}
		segmentChangeFetcherLatencies.RegisterLatency("segmentChangeFetcher.time", startTime)
		segmentChangeFetcherLatencies.RegisterLatency("backend::/api/segmentChanges", startTime)
		segmentChangeFetcherCounters.Increment("segmentChangeFetcher.status.200")
		segmentChangeFetcherLocalCounters.Increment("backend::request.ok")
		log.Debug.Println(">>>> Fetched segment:", segment.Name)

		if lastChangeNumber >= segment.Till {
			log.Debug.Println("Segments returned by the server are empty")
			//Unlock channel
			<-blocker
			return
		}

		//updating change number
		j.segmentStorage.SetChangeNumber(segment.Name, segment.Till)

		//adding new keys to segment
		if err := j.segmentStorage.AddToSegment(segment.Name, segment.Added); err != nil {
			log.Error.Printf("Error adding keys to segment %s", segment.Name)
		}

		//removing keys from segment
		if err := j.segmentStorage.RemoveFromSegment(segment.Name, segment.Removed); err != nil {
			log.Error.Printf("Error removing keys from segment %s", segment.Name)
		}

		time.Sleep(time.Millisecond * 500)
	}

	<-blocker
	return
}

// worker to run jobs
func worker() {
	for {
		_job := <-jobs
		go _job.run()
	}
}

// FetchSegments task to retrieve segments changes from Split servers
func FetchSegments(segmentFetcherAdapter fetcher.SegmentFetcherFactory,
	storageAdapterFactory storage.SegmentStorageFactory,
	fetchRate int, wg *sync.WaitGroup) {

	wg.Add(1)

	//TODO Set blocker channel size by configuration
	blocker = make(chan bool, 10)

	jobs = make(chan job)
	var jobsPool = make(map[string]*job)

	// worker to fetch jobs and run it.
	go worker()

	storageAdapter := storageAdapterFactory.NewInstance()
	keepLoop := true
	for keepLoop {
		segmentsNames, err := storageAdapter.RegisteredSegmentNames()
		if err != nil {
			log.Error.Println("Error fetching segments from storage", err.Error())
			keepLoop = !stopSignal(time.Second * 30)
			continue
		}
		log.Verbose.Printf("Fetched Segments from storage: %s", segmentsNames)

		taskFetchSegments(jobsPool, segmentsNames, segmentFetcherAdapter, storageAdapterFactory)

		jobsWaitingGroup.Wait()

		keepLoop = !stopSignal(time.Duration(fetchRate) * time.Second)
	}

	wg.Done()
}

func stopSignal(waitFor time.Duration) bool {
	select {
	case msg := <-segmentsIncoming:
		if msg == "STOP" {
			log.Debug.Println("Stopping task: fetch_segments")
			return true
		}
	case <-time.After(waitFor):
	}
	return false
}

func taskFetchSegments(jobsPool map[string]*job, segmentsNames []string, segmentFetcherAdapter fetcher.SegmentFetcherFactory,
	storageAdapterFactory storage.SegmentStorageFactory) {
	for i := 0; i < len(segmentsNames); i++ {
		if jobsPool[segmentsNames[i]] == nil {
			jobsPool[segmentsNames[i]] = &job{segmentName: segmentsNames[i],
				segmentFetcher: segmentFetcherAdapter.NewInstance(),
				segmentStorage: storageAdapterFactory.NewInstance()}
		}
	}

	// Running jobs in waiting group
	for _, v := range jobsPool {
		// Increment the WaitGroup counter.
		jobsWaitingGroup.Add(1)
		//go v.run()
		jobs <- *v
	}
}
