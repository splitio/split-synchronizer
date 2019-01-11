package task

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/stats/counter"
	"github.com/splitio/split-synchronizer/splitio/stats/latency"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

const maxBulkSizeImpressions = int64(5000)

var impressionsIncoming chan string

// InitializeImpressions initialiaze events task
func InitializeImpressions(threads int) {
	impressionsIncoming = make(chan string, threads)
}

// StopPostImpressions stops PostImpressions task sendding signal
func StopPostImpressions() {
	select {
	case impressionsIncoming <- "STOP":
	default:
	}
}

// ImpressionBulk struct
type ImpressionBulk struct {
	Data        json.RawMessage
	SdkVersion  string
	MachineIP   string
	MachineName string
	attempt     int
}

var testImpressionsLatencies = latency.NewLatencyBucket()
var testImpressionsCounters = counter.NewCounter()
var testImpressionsLocalCounters = counter.NewCounter()

var mutex = &sync.Mutex{}

func taskPostImpressions(
	tid int,
	impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter storage.ImpressionStorage,
	impressionsPerPost int64,
	legacyDisabled bool,
	impressionListenerEnabled bool,
) {

	mutex.Lock()
	beforeHitRedis := time.Now().UnixNano()
	impressionsToSend, err := impressionStorageAdapter.RetrieveImpressions(impressionsPerPost, legacyDisabled)
	afterHitRedis := time.Now().UnixNano()
	tookHitRedis := afterHitRedis - beforeHitRedis
	log.Benchmark.Println("Redis Request took", tookHitRedis)
	mutex.Unlock()

	if err != nil {
		log.Error.Println("Error Retrieving ")
	} else {
		log.Verbose.Println(impressionsToSend)
		for metadata, impressions := range impressionsToSend {
			log.Debug.Printf("Posting impressions for metadata: %+v ", metadata)
			beforePostServer := time.Now().UnixNano()
			startTime := testImpressionsLatencies.StartMeasuringLatency()
			err = impressionsRecorderAdapter.Post(impressions, metadata)
			if err != nil {
				log.Error.Println("Error posting impressions to split backend", err.Error())

				if _, ok := err.(*api.HttpError); ok {
					testImpressionsLocalCounters.Increment("backend::request.error")
					testImpressionsCounters.Increment(fmt.Sprintf("testImpressions.status.%d", err.(*api.HttpError).Code))
				}

			} else {
				log.Benchmark.Println("POST impressions to Server took", (time.Now().UnixNano() - beforePostServer))
				log.Debug.Println("Impressions sent")
				testImpressionsCounters.Increment("testImpressions.status.200")
				testImpressionsLatencies.RegisterLatency("backend::/api/testImpressions/bulk", startTime)
				testImpressionsLatencies.RegisterLatency("testImpressions.time", startTime)
				testImpressionsLocalCounters.Increment("backend::request.ok")
			}
			if impressionListenerEnabled {
				rawImpressions, err := json.Marshal(impressions)
				if err != nil {
					log.Error.Println("JSON encoding failed for the following impressions", impressions)
					continue
				}
				err = QueueImpressionsForListener(&ImpressionBulk{
					Data:        json.RawMessage(rawImpressions),
					SdkVersion:  metadata.SdkVersion,
					MachineIP:   metadata.MachineIP,
					MachineName: metadata.MachineName,
				})
				if err != nil {
					log.Error.Println(err)
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
	legacyDisabled bool,
	impressionListenerEnabled bool,
	impressionsPerPost int64,
	wg *sync.WaitGroup,
) {
	wg.Add(1)
	keepLoop := true
	for keepLoop {
		taskPostImpressions(
			tid,
			impressionsRecorderAdapter,
			impressionStorageAdapter,
			impressionsPerPost,
			legacyDisabled,
			impressionListenerEnabled,
		)

		select {
		case msg := <-impressionsIncoming:
			if msg == "STOP" {
				log.Debug.Println("Stopping task: post_impressions")
				keepLoop = false
			}
		case <-time.After(time.Duration(impressionsRefreshRate) * time.Second):
		}
	}
	wg.Done()
}

// ImpressionsFlush Task to flush cached impressions.
func ImpressionsFlush(
	impressionsRecorderAdapter recorder.ImpressionsRecorder,
	impressionStorageAdapter *redis.ImpressionStorageAdapter,
	size *int64,
	legacyDisabled,
	impressionListenerEnabled bool,
) {
	if size == nil {
		for impressionStorageAdapter.Size(impressionStorageAdapter.GetQueueNamespace()) > 0 {
			taskPostImpressions(
				0,
				impressionsRecorderAdapter,
				impressionStorageAdapter,
				maxBulkSizeImpressions,
				legacyDisabled,
				impressionListenerEnabled,
			)
		}
	} else {
		elementsToFlush := *size
		for elementsToFlush > 0 && impressionStorageAdapter.Size(impressionStorageAdapter.GetQueueNamespace()) > 0 {
			maxSize := maxBulkSizeImpressions
			if elementsToFlush < maxBulkSizeImpressions {
				maxSize = elementsToFlush
			}
			taskPostImpressions(
				0,
				impressionsRecorderAdapter,
				impressionStorageAdapter,
				maxSize,
				legacyDisabled,
				impressionListenerEnabled,
			)
			elementsToFlush = elementsToFlush - maxBulkSizeImpressions
		}
	}
}
