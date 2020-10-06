package controllers

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/service/api"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-split-commons/v2/util"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/interfaces"
)

//-----------------------------------------------------------------
// IMPRESSIONS
//-----------------------------------------------------------------
type impressionsModeBuffer map[string][][]byte
type machineNameBuffer map[string]impressionsModeBuffer
type machineIPBuffer map[string]machineNameBuffer
type sdkVersionBuffer map[string]machineIPBuffer

const impressionChannelCapacity = 5

var impressionPoolBuffer = make(sdkVersionBuffer)

var impressionPoolBufferSize = impressionPoolBufferSizeStruct{size: 0}
var impressionCurrentPoolBucket = 0
var impressionMutexPoolBuffer = sync.Mutex{}
var impressionChannel = make(chan impressionChanMessage, impressionChannelCapacity)
var impressionPoolBufferChannel = make(chan int, 10)
var impressionWorkersStopChannel = make(chan bool, impressionChannelCapacity)

const impressionChannelMessageRelease = 0
const impressionChannelMessageStop = 10

var impressionRecorder *api.HTTPImpressionRecorder

//----------------------------------------------------------------
//----------------------------------------------------------------
type impressionPoolBufferSizeStruct struct {
	sync.RWMutex
	size int64
}

func (s *impressionPoolBufferSizeStruct) Addition(v int64) {
	s.Lock()
	s.size += v
	s.Unlock()
}

func (s *impressionPoolBufferSizeStruct) Reset() {
	s.Lock()
	s.size = 0
	s.Unlock()
}

func (s *impressionPoolBufferSizeStruct) GreaterThan(v int64) bool {
	s.RLock()
	if s.size > v {
		s.RUnlock()
		return true
	}
	s.RUnlock()
	return false
}

//----------------------------------------------------------------
//----------------------------------------------------------------

type impressionChanMessage struct {
	SdkVersion      string
	MachineIP       string
	MachineName     string
	ImpressionsMode string
	Data            []byte
}

// InitializeImpressionWorkers initializes impression workers
func InitializeImpressionWorkers(footprint int64, postRate int64, waitingGroup *sync.WaitGroup) {
	impressionRecorder = api.NewHTTPImpressionRecorder(conf.Data.APIKey, conf.ParseAdvancedOptions(), log.Instance)
	go impressionConditionsWorker(postRate, waitingGroup)
	for i := 0; i < impressionChannelCapacity; i++ {
		go addImpressionsToBufferWorker(footprint, waitingGroup)
	}
}

// AddImpressions non-blocking function to add impressions and return response
func AddImpressions(data []byte, sdkVersion string, machineIP string, machineName string, impressionsMode string) {
	impressionChannel <- impressionChanMessage{SdkVersion: sdkVersion,
		MachineIP: machineIP, MachineName: machineName, Data: data, ImpressionsMode: strings.ToLower(impressionsMode)}
}

func impressionConditionsWorker(postRate int64, waitingGroup *sync.WaitGroup) {
	waitingGroup.Add(1)
	defer waitingGroup.Done()
	idleDuration := time.Second * time.Duration(postRate)
	timer := time.NewTimer(idleDuration)
	for {
		timer.Reset(idleDuration)
		// Blocking conditions to send impressions
		select {
		case msg := <-impressionPoolBufferChannel:
			switch msg {
			case impressionChannelMessageRelease:
				log.Instance.Debug("Releasing impressions by Size")
			case impressionChannelMessageStop:
				// flush impressions and finish
				sendImpressions()
				return
			}
		case <-timer.C:
			log.Instance.Debug("Releasing impressions by post rate")
		}

		sendImpressions()
	}
}

func addImpressionsToBufferWorker(footprint int64, waitingGroup *sync.WaitGroup) {
	waitingGroup.Add(1)
	defer waitingGroup.Done()
	for {
		var impMessage impressionChanMessage
		select {
		case <-impressionWorkersStopChannel:
			return
		case impMessage = <-impressionChannel:
		}

		data := impMessage.Data
		sdkVersion := impMessage.SdkVersion
		machineIP := impMessage.MachineIP
		machineName := impMessage.MachineName
		impressionsMode := impMessage.ImpressionsMode

		impressionMutexPoolBuffer.Lock()
		//Update current buffer size
		dataSize := len(data)
		impressionPoolBufferSize.Addition(int64(dataSize))

		if impressionPoolBuffer[sdkVersion] == nil {
			impressionPoolBuffer[sdkVersion] = make(machineIPBuffer)
		}

		if impressionPoolBuffer[sdkVersion][machineIP] == nil {
			impressionPoolBuffer[sdkVersion][machineIP] = make(machineNameBuffer)
		}

		if impressionPoolBuffer[sdkVersion][machineIP][machineName] == nil {
			impressionPoolBuffer[sdkVersion][machineIP][machineName] = make(impressionsModeBuffer)
		}

		if impressionPoolBuffer[sdkVersion][machineIP][machineName][impressionsMode] == nil {
			impressionPoolBuffer[sdkVersion][machineIP][machineName][impressionsMode] = make([][]byte, 0)
		}

		impressionPoolBuffer[sdkVersion][machineIP][machineName][impressionsMode] = append(impressionPoolBuffer[sdkVersion][machineIP][machineName][impressionsMode], data)

		impressionMutexPoolBuffer.Unlock()

		if impressionPoolBufferSize.GreaterThan(footprint) {
			impressionPoolBufferChannel <- impressionChannelMessageRelease
		}
	}

}

func sendImpressions() {
	impressionMutexPoolBuffer.Lock()
	impressionPoolBufferSize.Reset()
	for sdkVersion, machineIPMap := range impressionPoolBuffer {
		for machineIP, machineMap := range machineIPMap {
			for machineName, impressionsModeMap := range machineMap {
				for impressionsMode, listImpressions := range impressionsModeMap {

					var toSend = make([]json.RawMessage, 0)

					for _, byteImpression := range listImpressions {
						var rawImpressions []json.RawMessage
						err := json.Unmarshal(byteImpression, &rawImpressions)
						if err != nil {
							log.Instance.Error(err)
							continue
						}

						for _, impression := range rawImpressions {
							toSend = append(toSend, impression)
						}

					}

					data, errl := json.Marshal(toSend)
					if errl != nil {
						log.Instance.Error(errl)
						continue
					}
					before := time.Now()
					var extraHeaders map[string]string
					if impressionsMode != "" {
						extraHeaders = make(map[string]string)
						extraHeaders["SplitSDKImpressionsMode"] = impressionsMode
					}
					errp := impressionRecorder.RecordRaw("/testImpressions/bulk",
						data, dtos.Metadata{
							SDKVersion:  sdkVersion,
							MachineIP:   machineIP,
							MachineName: machineName,
						}, extraHeaders)
					if errp != nil {
						log.Instance.Error(errp)
						if httpError, ok := errp.(*dtos.HTTPError); ok {
							interfaces.ProxyTelemetryWrapper.StoreCounters(storage.TestImpressionsCounter, string(httpError.Code))
						}
					} else {
						bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
						interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.TestImpressionsLatency, bucket)
						interfaces.ProxyTelemetryWrapper.StoreCounters(storage.TestImpressionsCounter, "ok")
					}
				}
			}
		}
	}
	// Clear the impressionPoolBuffer
	impressionPoolBuffer = make(sdkVersionBuffer)
	impressionMutexPoolBuffer.Unlock()
}

// StopImpressionsRecording stops all tasks related to impression submission.
func StopImpressionsRecording() {
	impressionPoolBufferChannel <- impressionChannelMessageStop
	for i := 0; i < impressionChannelCapacity; i++ {
		impressionWorkersStopChannel <- true
	}
}
