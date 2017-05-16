package task

import (
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
)

var mutex = &sync.Mutex{}

func taskPostImpressions(tid int, impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage) {
	mutex.Lock()
	impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions()
	mutex.Unlock()
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
}

// PostImpressions post impressions to Split Events server
func PostImpressions(tid int, impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionsRefreshRate int) {
	for {
		taskPostImpressions(tid, impressionsRecorderAdapter, impressionStorageAdapter)
		time.Sleep(time.Duration(impressionsRefreshRate) * time.Second)
	}

}
