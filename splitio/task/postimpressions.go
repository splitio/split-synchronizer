// Package task contains all agent tasks
package task

import (
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
)

// PostImpressions post impressions to Split Events server
func PostImpressions(impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionsRefreshRate int) {
	for {
		impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions()
		if err != nil {
			log.Error.Println("Error Retrieving ")
		} else {
			log.Verbose.Println(impressionsToSend)

			for sdkVersion, impressionsByMachineIP := range impressionsToSend {
				for machineIP, impressions := range impressionsByMachineIP {
					log.Debug.Println("Posting impressions from ", sdkVersion, machineIP)
					err := impressionsRecorderAdapter.Post(impressions, sdkVersion, machineIP)
					if err != nil {
						log.Error.Println("Error posting impressions", err.Error())
						continue
					}
					log.Debug.Println("Impressions sent")
				}
			}
		}

		time.Sleep(time.Duration(impressionsRefreshRate) * time.Second)
	}

}
