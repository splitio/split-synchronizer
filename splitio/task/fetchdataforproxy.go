package task

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"
)

var proxyFetchSegmentBlocker chan bool
var proxyInProgressSegments map[string]struct{}
var proxySegmentToProcess chan string
var mutexInProgress = &sync.Mutex{}

// worker to fetch segments
func proxyFetchSegmentsWorker() {
	for {
		segmentName := <-proxySegmentToProcess
		mutexInProgress.Lock()
		if _, ok := proxyInProgressSegments[segmentName]; !ok {
			proxyInProgressSegments[segmentName] = struct{}{}
			go fetchSegment(segmentName)
		}
		mutexInProgress.Unlock()
	}
}

func fetchSegment(segment string) {
	fmt.Println(segment)
	//time.Sleep(time.Duration(10) * time.Second)
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	var since int64 = -1
	for {
		rawData, err := api.SegmentChangesFetchRaw(segment, since)
		if err != nil {
			log.Error.Println("Error fetching split changes ", err)
			break
		}

		log.Verbose.Println(string(rawData))

		segmentChangesItem := &collections.SegmentChangesItem{}
		err = json.Unmarshal(rawData, segmentChangesItem)
		if err != nil {
			log.Error.Println("Error parsing segment changes JSON ", err)
			break
		}
		// Adding rawData into JSON attribute.
		segmentChangesItem.JSON = rawData

		//Saving in memory db
		err = segmentCollection.Add(segmentChangesItem)
		if err != nil {
			log.Error.Println("Error saving segment", err)
			break
		}

		if segmentChangesItem.Since >= segmentChangesItem.Till {
			break
		} else {
			since = segmentChangesItem.Till
		}

	}

	// Release the in-progress segment
	mutexInProgress.Lock()
	delete(proxyInProgressSegments, segment)
	mutexInProgress.Unlock()
}

// FetchRawSplits task to retrieve split changes from Split servers
func FetchRawSplits(splitsRefreshRate int) {
	// Initialize global variables
	proxyFetchSegmentBlocker = make(chan bool, 10)
	proxyInProgressSegments = make(map[string]struct{}, 0)
	proxySegmentToProcess = make(chan string)

	//Launch fetch segments worker
	go proxyFetchSegmentsWorker()

	// Starting to fetch splits
	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)

	//TODO fetch last since from collection
	var since int64 = -1

	for {
		rawData, err := api.SplitChangesFetchRaw(since)
		if err != nil {
			log.Error.Println("Error fetching split changes ", err)
			continue
		}

		log.Verbose.Println(string(rawData))

		splitChangesItem := &collections.SplitChangesItem{}
		err = json.Unmarshal(rawData, splitChangesItem)
		if err != nil {
			log.Error.Println("Error parsing split changes JSON ", err)
			continue
		}
		// Adding rawData into JSON attribute.
		splitChangesItem.JSON = rawData

		//Saving in memory db
		err = splitCollection.Add(splitChangesItem)
		if err != nil {
			log.Error.Println(err)
			continue
		}

		//Fetching segments
		retrieveSegments(rawData)

		//update since for next call
		since = splitChangesItem.Till

		time.Sleep(time.Duration(splitsRefreshRate) * time.Second)
	}
}

func retrieveSegments(rawData []byte) {
	var splitChangesDto api.SplitChangesDTO
	err := json.Unmarshal(rawData, &splitChangesDto)
	if err != nil {
		log.Error.Println("Error parsing split changes JSON ", err)
	}

	splits := splitChangesDto.Splits
	totalItems := len(splits)

	for i := 0; i < totalItems; i++ {
		totalConditions := len(splits[i].Conditions)
		for j := 0; j < totalConditions; j++ {
			totalMatchers := len(splits[i].Conditions[j].MatcherGroup.Matchers)
			for k := 0; k < totalMatchers; k++ {
				if splits[i].Conditions[j].MatcherGroup.Matchers[k].MatcherType == "IN_SEGMENT" {
					segmentName := splits[i].Conditions[j].MatcherGroup.Matchers[k].UserDefinedSegment.SegmentName
					log.Debug.Println("Fetching Segment:", segmentName)
					// Adding segment to channel to be processed by worker
					proxySegmentToProcess <- segmentName
				}
			}
		}
	}
}
