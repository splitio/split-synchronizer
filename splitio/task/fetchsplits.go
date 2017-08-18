package task

import (
	"encoding/json"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

func taskFetchSplits(splitFetcherAdapter fetcher.SplitFetcher,
	splitStorageAdapter storage.SplitStorage) {

	lastChangeNumber, err := splitStorageAdapter.ChangeNumber()
	if err != nil {
		log.Debug.Printf("Fetching change number: %s\n", err.Error())
		lastChangeNumber = -1
	}

	data, err := splitFetcherAdapter.Fetch(lastChangeNumber)
	if err != nil {
		log.Error.Println("Error fetching SplitDTO on task ", err.Error())
	} else {
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
	splitsRefreshRate int) {
	for {
		taskFetchSplits(splitFetcherAdapter, splitStorageAdapter)
		time.Sleep(time.Duration(splitsRefreshRate) * time.Second)
	}
}
