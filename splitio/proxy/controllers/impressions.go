// Package controllers implements functions to call from http controllers
package controllers

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/stats/counter"
	"github.com/splitio/split-synchronizer/splitio/stats/latency"
)

var impressionLatencyRegister = latency.NewLatencyBucket()
var impressionCounterRegister = counter.NewLocalCounter()

//-----------------------------------------------------------------
// IMPRESSIONS
//-----------------------------------------------------------------
type machineNameBuffer map[string][][]byte
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
	SdkVersion  string
	MachineIP   string
	MachineName string
	Data        []byte
}

// InitializeImpressionWorkers initializes impression workers
func InitializeImpressionWorkers(footprint int64, postRate int64, waitingGroup *sync.WaitGroup) {
	go impressionConditionsWorker(postRate, waitingGroup)
	for i := 0; i < impressionChannelCapacity; i++ {
		go addImpressionsToBufferWorker(footprint, waitingGroup)
	}
}

// AddImpressions non-blocking function to add impressions and return response
func AddImpressions(data []byte, sdkVersion string, machineIP string, machineName string) {
	var imp = impressionChanMessage{SdkVersion: sdkVersion,
		MachineIP: machineIP, MachineName: machineName, Data: data}

	impressionChannel <- imp
}

func impressionConditionsWorker(postRate int64, waitingGroup *sync.WaitGroup) {
	waitingGroup.Add(1)
	defer waitingGroup.Done()
	for {
		// Blocking conditions to send impressions
		select {
		case msg := <-impressionPoolBufferChannel:
			switch msg {
			case impressionChannelMessageRelease:
				log.Debug.Println("Releasing impressions by Size")
			case impressionChannelMessageStop:
				// flush impressions and finish
				sendImpressions()
				return
			}
		case <-time.After(time.Second * time.Duration(postRate)):
			log.Debug.Println("Releasing impressions by post rate")
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
			impressionPoolBuffer[sdkVersion][machineIP][machineName] = make([][]byte, 0)
		}

		impressionPoolBuffer[sdkVersion][machineIP][machineName] = append(impressionPoolBuffer[sdkVersion][machineIP][machineName], data)

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
			for machineName, listImpressions := range machineMap {

				var toSend = make([]json.RawMessage, 0)

				for _, byteImpression := range listImpressions {
					var rawImpressions []json.RawMessage
					err := json.Unmarshal(byteImpression, &rawImpressions)
					if err != nil {
						log.Error.Println(err)
						continue
					}

					for _, impression := range rawImpressions {
						toSend = append(toSend, impression)
					}

				}

				data, errl := json.Marshal(toSend)
				if errl != nil {
					log.Error.Println(errl)
					continue
				}
				startCheckpoint := impressionLatencyRegister.StartMeasuringLatency()
				errp := api.PostImpressions(data, sdkVersion, machineIP, machineName)
				if errp != nil {
					log.Error.Println(errp)
					impressionCounterRegister.Increment("backend::request.error")
				} else {
					impressionLatencyRegister.RegisterLatency("backend::/api/testImpressions/bulk", startCheckpoint)
					impressionCounterRegister.Increment("backend::request.ok")
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
