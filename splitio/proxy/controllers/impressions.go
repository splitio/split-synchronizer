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

var latencyRegister = latency.NewLatencyBucket()
var counterRegister = counter.NewLocalCounter()

//-----------------------------------------------------------------
// IMPRESSIONS
//-----------------------------------------------------------------
type machineNameBuffer map[string][][]byte
type machineIPBuffer map[string]machineNameBuffer
type sdkVersionBuffer map[string]machineIPBuffer

const impressionChannelCapacity = 5

var poolBuffer = make(sdkVersionBuffer)

var poolBufferSize = poolBufferSizeStruct{size: 0}
var currentPoolBucket = 0
var mutexPoolBuffer = sync.Mutex{}
var impressionChannel = make(chan impressionChanMessage, impressionChannelCapacity)
var poolBufferReleaseChannel = make(chan bool, 1)

//----------------------------------------------------------------
//----------------------------------------------------------------
type poolBufferSizeStruct struct {
	sync.RWMutex
	size int64
}

func (s *poolBufferSizeStruct) Addition(v int64) {
	s.Lock()
	s.size += v
	s.Unlock()
}

func (s *poolBufferSizeStruct) Reset() {
	s.Lock()
	s.size = 0
	s.Unlock()
}

func (s *poolBufferSizeStruct) GreaterThan(v int64) bool {
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

// Initialize workers
func Initialize(footprint int64, postRate int64) {
	go conditionsWorker(postRate)
	for i := 0; i < impressionChannelCapacity; i++ {
		go addImpressionsToBufferWorker(footprint)
	}
}

// AddImpressions non-blocking function to add impressions and return response
func AddImpressions(data []byte, sdkVersion string, machineIP string, machineName string) {
	var imp = impressionChanMessage{SdkVersion: sdkVersion,
		MachineIP: machineIP, MachineName: machineName, Data: data}

	impressionChannel <- imp
}

func conditionsWorker(postRate int64) {
	for {
		// Blocking conditions to send impressions
		select {
		case <-poolBufferReleaseChannel:
			log.Debug.Println("Releasing impressions by Size")
		case <-time.After(time.Second * time.Duration(postRate)):
			log.Debug.Println("Releasing impressions by post rate")
		}

		sendImpressions()
	}
}

func addImpressionsToBufferWorker(footprint int64) {

	for {
		impMessage := <-impressionChannel

		data := impMessage.Data
		sdkVersion := impMessage.SdkVersion
		machineIP := impMessage.MachineIP
		machineName := impMessage.MachineName

		mutexPoolBuffer.Lock()
		//Update current buffer size
		dataSize := len(data)
		poolBufferSize.Addition(int64(dataSize))

		if poolBuffer[sdkVersion] == nil {
			poolBuffer[sdkVersion] = make(machineIPBuffer)
		}

		if poolBuffer[sdkVersion][machineIP] == nil {
			poolBuffer[sdkVersion][machineIP] = make(machineNameBuffer)
		}

		if poolBuffer[sdkVersion][machineIP][machineName] == nil {
			poolBuffer[sdkVersion][machineIP][machineName] = make([][]byte, 0)
		}

		poolBuffer[sdkVersion][machineIP][machineName] = append(poolBuffer[sdkVersion][machineIP][machineName], data)

		mutexPoolBuffer.Unlock()

		if poolBufferSize.GreaterThan(footprint) {
			poolBufferReleaseChannel <- true
		}
	}

}

func sendImpressions() {
	mutexPoolBuffer.Lock()
	poolBufferSize.Reset()
	for sdkVersion, machineIPMap := range poolBuffer {
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
				startCheckpoint := latencyRegister.StartMeasuringLatency()
				errp := api.PostImpressions(data, sdkVersion, machineIP, machineName)
				if errp != nil {
					log.Error.Println(errp)
					counterRegister.Increment("backend::request.error")
				} else {
					latencyRegister.RegisterLatency("backend::/api/testImpressions/bulk", startCheckpoint)
					counterRegister.Increment("backend::request.ok")
				}

			}
		}
	}
	// Clear the poolBuffer
	poolBuffer = make(sdkVersionBuffer)
	mutexPoolBuffer.Unlock()
}
