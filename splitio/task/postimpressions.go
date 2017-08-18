package task

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
)

type ImpressionBulk struct {
	Data        json.RawMessage
	SdkVersion  string
	MachineIP   string
	MachineName string
	attempt     int
}

var mutex = &sync.Mutex{}

func taskPostImpressions(
	tid int,
	impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionListenerEnabled bool,
) {

	mutex.Lock()
	beforeHitRedis := time.Now().UnixNano()
	impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions()
	afterHitRedis := time.Now().UnixNano()
	tookHitRedis := afterHitRedis - beforeHitRedis
	log.Benchmark.Println("Redis Request took", tookHitRedis)
	mutex.Unlock()

	if err != nil {
		log.Error.Println("Error Retrieving ")
	} else {
		log.Verbose.Println(impressionsToSend)

		for sdkVersion, impressionsByMachineIP := range impressionsToSend {
			for machineIP, impressions := range impressionsByMachineIP {
				log.Debug.Println("Posting impressions from ", sdkVersion, machineIP)
				beforePostServer := time.Now().UnixNano()
				err = impressionsRecorderAdapter.Post(impressions, sdkVersion, machineIP, "")
				if err != nil {
					log.Error.Println("Error posting impressions to split backend", err.Error())
				} else {
					log.Benchmark.Println("POST impressions to Server took", (time.Now().UnixNano() - beforePostServer))
					log.Debug.Println("Impressions sent")
				}
				if impressionListenerEnabled {
					rawImpressions, err := json.Marshal(impressions)
					if err != nil {
						log.Error.Println("JSON encoding failed for the following impressions", impressions)
						continue
					}
					err = QueueImpressionsForListener(&ImpressionBulk{
						Data:        json.RawMessage(rawImpressions),
						SdkVersion:  sdkVersion,
						MachineIP:   machineIP,
						MachineName: "",
					})
					if err != nil {
						log.Error.Println(err)
					}
				}
			}
		}
	}
}

// PostImpressions post impressions to Split Events server
func PostImpressions(
	tid int,
	impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionsRefreshRate int,
	impressionListenerEnabled bool,
) {
	for {
		taskPostImpressions(
			tid,
			impressionsRecorderAdapter,
			impressionStorageAdapter,
			impressionListenerEnabled,
		)

		time.Sleep(time.Duration(impressionsRefreshRate) * time.Second)
	}

}
