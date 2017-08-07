package task

import (
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
	"github.com/splitio/go-agent/splitio/util"
)

type impressionBulk struct {
	data        []api.ImpressionsDTO
	sdkVersion  string
	machineIP   string
	machineName string
	attempt     int
}

var ImpressionListenerEnabled = false
var impressionListenerStream = make(chan impressionBulk, util.ImpressionListenerMainQueueSize)
var mutex = &sync.Mutex{}

func taskPostImpressions(tid int, impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage) {

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
				err := impressionsRecorderAdapter.Post(impressions, sdkVersion, machineIP, "")
				if ImpressionListenerEnabled {
					select {
					case impressionListenerStream <- impressionBulk{
						data:        impressions,
						sdkVersion:  sdkVersion,
						machineIP:   machineIP,
						machineName: "",
					}:
					default:
						log.Error.Println("Impression listenser send queue is full, " +
							"impressions not sent to listener")
					}

				}
				if err != nil {
					log.Error.Println("Error posting impressions", err.Error())
					continue
				}
				log.Benchmark.Println("POST impressions to Server took", (time.Now().UnixNano() - beforePostServer))
				log.Debug.Println("Impressions sent")
			}
		}
	}
}

func PostImpressionsToListener(impressionsRecorderAdapter recorder.ImpressionsRecorder) {
	var failedQueue = make(chan impressionBulk, util.ImpressionListenerFailedQueueSize)
	for {
		failedImpressions := true
		for failedImpressions {
			select {
			case msg := <-failedQueue:
				err := impressionsRecorderAdapter.Post(msg.data, msg.sdkVersion, msg.machineIP, "")
				if err != nil {
					msg.attempt++
					if msg.attempt < 3 {
						failedQueue <- msg
					}
					time.Sleep(time.Millisecond * 100)
				}
			default:
				failedImpressions = false
			}
		}
		msg := <-impressionListenerStream
		err := impressionsRecorderAdapter.Post(msg.data, msg.sdkVersion, msg.machineIP, msg.machineName)
		if err != nil {
			select {
			case failedQueue <- msg:
			default:
				log.Error.Println("Impression listener queue is full. " +
					"Impressions will be dropped until the listener enpoint is restored.")
			}
			time.Sleep(time.Millisecond * 100)
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
