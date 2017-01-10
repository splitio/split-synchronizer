// Package task contains all agent tasks
package task

import (
	"time"

	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/storage"
)

// SplitFetcher task to retrieve split changes from Split servers
func SplitFetcher(splitFetcherAdapter fetcher.SplitFetcher, splitStorageAdapter storage.SplitStorage) {
	// splitFetcherAdapter Could be a fetcher from local file or from HTTP
	// splitStorageAdapter Could be Redis adapter or MySQL or in-memory

	for {
		data, err := splitFetcherAdapter.Fetch()
		if errors.IsError(err) {
			log.Error.Println("Error fetching SplitDTO on task ", err.Error())
		} else {
			log.Verbose.Println(data)
			//TODO Save data in Storage ---> following spec(???)

			totalItems := len(data)
			for i := 0; i < totalItems; i++ {
				splitStorageAdapter.Save(data[i].Name, data[i])
			}
			log.Info.Println("All splits have been saved.", " Total items: ", totalItems)
		}

		//TODO set time via config
		time.Sleep(15 * time.Second)
	}
}
