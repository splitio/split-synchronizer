// Package task contains all agent tasks
package task

import (
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/storage"
)

var blocker chan bool
var jobs chan job

type job struct {
	segmentName    string
	segmentFetcher fetcher.SegmentFetcher
}

func (j job) run() {
	blocker <- true

	//log.Debug.Println("Segment", j.segmentName)
	segment, errSegmentFetch := j.segmentFetcher.Fetch(j.segmentName)
	if errSegmentFetch != nil {
		log.Error.Println("Error fetching segment ", j.segmentName, errSegmentFetch.Error())
	}
	log.Info.Println(segment.Name)

	time.Sleep(time.Second * 5)

	<-blocker
}

func worker() {
	for {
		_job := <-jobs
		go _job.run()
	}
}

// FetchSegments task to retrieve segments changes from Split servers
func FetchSegments(segmentFetcherAdapter fetcher.SegmentFetcherFactory, storageAdapter storage.SegmentStorage) {

	blocker = make(chan bool, 10)
	jobs = make(chan job)

	segmentsNames, err := storageAdapter.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println("Error fetching segments from storage", err.Error())
	}
	log.Verbose.Printf("Fetched Segments from storage: %s", segmentsNames)
	/*
		segment, errSegmentFetch := segmentFetcherAdapter.Fetch(segmentsNames[0])
		if errSegmentFetch != nil {
			log.Error.Println("Error fetching segment ", segmentsNames[0], errSegmentFetch.Error())
		}
		log.Info.Println(segment.Name)
	*/
	go worker()

	for i := 0; i < len(segmentsNames); i++ {
		jobs <- job{segmentName: segmentsNames[i], segmentFetcher: segmentFetcherAdapter.NewInstance()}
	}

	// TODO for each segmentName trigger a go func to fetch segments
	/*
			  for {
			    for i:=0; i < len(segmentsNames); i++ {
		        go fetchSegmentData(segmentsNames[i])
			    }

			    time.Sleep(time.Duration(conf.Data.SegmentFetchRate) * time.Second)
			  }

	*/

}

/*
func fetchSegmentData(name string) {
	jj := job{segmentName: name}
	segmentPool <- jj
}
*/
