// Package task contains all agent tasks
package task

import (
	"time"

	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/storage"
)

// FetchSplits task to retrieve split changes from Split servers
func FetchSplits(splitFetcherAdapter fetcher.SplitFetcher,
	splitStorageAdapter storage.SplitStorage,
	splitsRefreshRate int) {
	for {
		lastChangeNumber, err := splitStorageAdapter.ChangeNumber()
		if err != nil {
			log.Debug.Printf("Fetching change number: %s\n", err.Error())
			lastChangeNumber = -1
		}

		data, err := splitFetcherAdapter.Fetch(lastChangeNumber)
		if errors.IsError(err) {
			log.Error.Println("Error fetching SplitDTO on task ", err.Error())
		} else {
			log.Verbose.Println(data)

			till := data.Till
			if errTill := splitStorageAdapter.SetChangeNumber(till); errTill != nil {
				log.Error.Println("Error saving till value into storage adapter.", errTill)
			}

			splits := data.Splits
			totalItems := len(splits)
			savedItems := 0
			deletedItems := 0
			for i := 0; i < totalItems; i++ {
				//if split is active then save it!
				if splits[i].Status == "ACTIVE" {
					if err := splitStorageAdapter.Save(splits[i]); err == nil {
						savedItems++
						totalConditions := len(splits[i].Conditions)
						for j := 0; j < totalConditions; j++ {
							totalMatchers := len(splits[i].Conditions[j].MatcherGroup.Matchers)
							for k := 0; k < totalMatchers; k++ {
								if splits[i].Conditions[j].MatcherGroup.Matchers[k].MatcherType == "IN_SEGMENT" {
									segmentName := splits[i].Conditions[j].MatcherGroup.Matchers[k].UserDefinedSegment.SegmentName
									if err := splitStorageAdapter.RegisterSegment(segmentName); err != nil {
										log.Error.Println("Error registering segment", segmentName, err)
									}
								}
							}
						}
					}
				} else {
					if err := splitStorageAdapter.Remove(splits[i]); err == nil {
						deletedItems++
					}
				}
			}
			log.Debug.Println("Saved splits:", savedItems, "Removed splits:", deletedItems, "of Total items: ", totalItems)
		}

		time.Sleep(time.Duration(splitsRefreshRate) * time.Second)
	}
}
