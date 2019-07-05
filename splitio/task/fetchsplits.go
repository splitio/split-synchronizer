package task

import (
	"encoding/json"
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

var splitsIncoming = make(chan string, 1)

// StopFetchSplits stops FetchSplits task sendding signal
func StopFetchSplits() {
	select {
	case splitsIncoming <- "STOP":
	default:
	}
}

var splitChangesLatencies = latency.NewLatencyBucket()
var splitChangesCounters = counter.NewCounter()
var splitChangesLocalCounters = counter.NewLocalCounter()

func taskFetchSplits(splitFetcherAdapter fetcher.SplitFetcher,
	splitStorageAdapter storage.SplitStorage) {

	lastChangeNumber, err := splitStorageAdapter.ChangeNumber()
	if err != nil {
		log.Debug.Printf("Fetching change number: %s\n", err.Error())
		lastChangeNumber = -1
	}

	if lastChangeNumber == -1 {
		splitStorageAdapter.CleanTrafficTypes()
	}

	startTime := splitChangesLatencies.StartMeasuringLatency()
	data, err := splitFetcherAdapter.Fetch(lastChangeNumber)
	if err != nil {
		log.Error.Println("Error fetching SplitDTO on task ", err.Error())

		if _, ok := err.(*api.HttpError); ok {
			splitChangesLocalCounters.Increment("backend::request.error")
			splitChangesCounters.Increment(fmt.Sprintf("splitChangeFetcher.status.%d", err.(*api.HttpError).Code))
		}
	} else {
		splitChangesLatencies.RegisterLatency("splitChangeFetcher.time", startTime)
		splitChangesLatencies.RegisterLatency("backend::/api/splitChanges", startTime)
		splitChangesCounters.Increment("splitChangeFetcher.status.200")
		splitChangesLocalCounters.Increment("backend::request.ok")
		log.Verbose.Println(data)

		till := data.Till
		if errTill := splitStorageAdapter.SetChangeNumber(till); errTill != nil {
			log.Error.Println("Error saving till value into storage adapter.", errTill)
		}

		rawSplits := data.RawSplits
		totalItemsRaw := len(data.RawSplits)
		savedItems := 0
		deletedItems := 0
		for i := 0; i < totalItemsRaw; i++ {
			// Decode Raw JSON
			var splitDTO api.SplitDTO
			if err = json.Unmarshal(*rawSplits[i], &splitDTO); err != nil {
				log.Error.Println(err)
				continue
			}
			jsonD, _ := rawSplits[i].MarshalJSON()
			if splitDTO.Status == "ACTIVE" {
				if err := splitStorageAdapter.Save(jsonD); err == nil {
					savedItems++
					totalConditions := len(splitDTO.Conditions)
					for j := 0; j < totalConditions; j++ {
						totalMatchers := len(splitDTO.Conditions[j].MatcherGroup.Matchers)
						for k := 0; k < totalMatchers; k++ {
							if splitDTO.Conditions[j].MatcherGroup.Matchers[k].MatcherType == "IN_SEGMENT" {
								segmentName := splitDTO.Conditions[j].MatcherGroup.Matchers[k].UserDefinedSegment.SegmentName
								if err := splitStorageAdapter.RegisterSegment(segmentName); err != nil {
									log.Error.Println("Error registering segment", segmentName, err)
								}
							}
						}
					}
				}
			} else {
				if err := splitStorageAdapter.Remove(jsonD); err == nil {
					deletedItems++
				}
			}
		}
		log.Debug.Println("Saved splits:", savedItems, "Removed splits:", deletedItems, "of Total items: ", totalItemsRaw)
	}
}

// FetchSplits task to retrieve split changes from Split servers
func FetchSplits(splitFetcherAdapter fetcher.SplitFetcher,
	splitStorageAdapter storage.SplitStorage,
	splitsRefreshRate int, wg *sync.WaitGroup) {
	wg.Add(1)
	keepLoop := true
	for keepLoop {
		taskFetchSplits(splitFetcherAdapter, splitStorageAdapter)

		select {
		case msg := <-splitsIncoming:
			if msg == "STOP" {
				log.Debug.Println("Stopping task: fetch_splits")
				keepLoop = false
			}
		case <-time.After(time.Duration(splitsRefreshRate) * time.Second):
		}
	}
	wg.Done()
}
