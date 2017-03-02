// Package task contains all agent tasks
package task

import (
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/storage"
)

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

		segment, errSegmentFetch := j.segmentFetcher.Fetch(j.segmentName, lastChangeNumber)
		if errSegmentFetch != nil {
			log.Error.Println("Error fetching segment ", j.segmentName, errSegmentFetch.Error())
			tryNumber--
			time.Sleep(time.Second * 5)
			continue
		}
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
	fetchRate int) {
	log.Debug.Println("FetchSegments refresh rate", fetchRate)
	blocker = make(chan bool, 10)
	jobs = make(chan job)
	var jobsPool = make(map[string]*job)

	storageAdapter := storageAdapterFactory.NewInstance()

	// worker to fetch jobs and run it.
	go worker()

	for {
		segmentsNames, err := storageAdapter.RegisteredSegmentNames()
		if err != nil {
			log.Error.Println("Error fetching segments from storage", err.Error())
			time.Sleep(time.Second * 30)
			continue
		}
		log.Verbose.Printf("Fetched Segments from storage: %s", segmentsNames)

		for i := 0; i < len(segmentsNames); i++ {
			if jobsPool[segmentsNames[i]] == nil {
				jobsPool[segmentsNames[i]] = &job{segmentName: segmentsNames[i],
					segmentFetcher: segmentFetcherAdapter.NewInstance(),
					segmentStorage: storageAdapterFactory.NewInstance()}
			}
			//jobs <- job{segmentName: segmentsNames[i], segmentFetcher: segmentFetcherAdapter.NewInstance(),segmentStorage: storageAdapterFactory.NewInstance()}
		}

		// Running jobs in waiting group
		for _, v := range jobsPool {
			// Increment the WaitGroup counter.
			jobsWaitingGroup.Add(1)
			//go v.run()
			jobs <- *v
		}

		jobsWaitingGroup.Wait()
		time.Sleep(time.Duration(fetchRate) * time.Second)
	}
}
