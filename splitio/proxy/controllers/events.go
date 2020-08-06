package controllers

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/proxy/interfaces"
)

const eventChannelCapacity = 5

var eventPoolBuffer = make(sdkVersionBuffer)

var eventPoolBufferSize = eventPoolBufferSizeStruct{size: 0}
var eventCurrentPoolBucket = 0
var eventMutexPoolBuffer = sync.Mutex{}
var eventChannel = make(chan eventChanMessage, eventChannelCapacity)
var eventWorkersStopChannel = make(chan bool, eventChannelCapacity)
var eventPoolBufferChannel = make(chan int, 10)

const eventChannelMessageRelease = 0
const eventChannelMessageStop = 10

var eventsRecorder = api.NewHTTPEventsRecorder(conf.Data.APIKey, interfaces.GetAdvancedConfig(), interfaces.Logger)

//----------------------------------------------------------------
//----------------------------------------------------------------
type eventPoolBufferSizeStruct struct {
	sync.RWMutex
	size int64
}

func (s *eventPoolBufferSizeStruct) Addition(v int64) {
	s.Lock()
	s.size += v
	s.Unlock()
}

func (s *eventPoolBufferSizeStruct) Reset() {
	s.Lock()
	s.size = 0
	s.Unlock()
}

func (s *eventPoolBufferSizeStruct) GreaterThan(v int64) bool {
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

type eventChanMessage struct {
	SdkVersion  string
	MachineIP   string
	MachineName string
	Data        []byte
}

// InitializeEventWorkers initializes event workers
func InitializeEventWorkers(footprint int64, postRate int64, waitingGroup *sync.WaitGroup) {
	go eventConditionsWorker(postRate, waitingGroup)
	for i := 0; i < eventChannelCapacity; i++ {
		go addEventsToBufferWorker(footprint, waitingGroup)
	}
}

// AddEvents non-blocking function to add events and return response
func AddEvents(data []byte, sdkVersion string, machineIP string, machineName string) {
	event := eventChanMessage{
		SdkVersion:  sdkVersion,
		MachineIP:   machineIP,
		MachineName: machineName,
		Data:        data,
	}

	eventChannel <- event
}

func eventConditionsWorker(postRate int64, waitingGroup *sync.WaitGroup) {
	waitingGroup.Add(1)
	defer waitingGroup.Done()
	for {
		// Blocking conditions to send events
		select {
		case msg := <-eventPoolBufferChannel:
			switch msg {
			case eventChannelMessageRelease:
				log.Debug.Println("Releasing events by Size")
			case eventChannelMessageStop:
				// flush events and finish
				sendEvents()
				return
			}
		case <-time.After(time.Second * time.Duration(postRate)):
			log.Debug.Println("Releasing events by post rate")
		}

		sendEvents()
	}
}

func addEventsToBufferWorker(footprint int64, waitingGroup *sync.WaitGroup) {
	waitingGroup.Add(1)
	defer waitingGroup.Done()

	for {
		var eventMessage eventChanMessage
		select {
		case <-eventWorkersStopChannel:
			return
		case eventMessage = <-eventChannel:
		}

		data := eventMessage.Data
		sdkVersion := eventMessage.SdkVersion
		machineIP := eventMessage.MachineIP
		machineName := eventMessage.MachineName

		eventMutexPoolBuffer.Lock()
		//Update current buffer size
		dataSize := len(data)
		eventPoolBufferSize.Addition(int64(dataSize))

		if eventPoolBuffer[sdkVersion] == nil {
			eventPoolBuffer[sdkVersion] = make(machineIPBuffer)
		}

		if eventPoolBuffer[sdkVersion][machineIP] == nil {
			eventPoolBuffer[sdkVersion][machineIP] = make(machineNameBuffer)
		}

		if eventPoolBuffer[sdkVersion][machineIP][machineName] == nil {
			eventPoolBuffer[sdkVersion][machineIP][machineName] = make([][]byte, 0)
		}

		eventPoolBuffer[sdkVersion][machineIP][machineName] = append(
			eventPoolBuffer[sdkVersion][machineIP][machineName],
			data,
		)

		eventMutexPoolBuffer.Unlock()

		if eventPoolBufferSize.GreaterThan(footprint) {
			eventPoolBufferChannel <- eventChannelMessageRelease
		}
	}

}

func sendEvents() {
	eventMutexPoolBuffer.Lock()
	defer eventMutexPoolBuffer.Unlock()

	eventPoolBufferSize.Reset()
	for sdkVersion, machineIPMap := range eventPoolBuffer {
		for machineIP, machineMap := range machineIPMap {
			for machineName, listEvents := range machineMap {
				var toSend = make([]json.RawMessage, 0)

				for _, byteEvent := range listEvents {
					var rawEvents []json.RawMessage
					err := json.Unmarshal(byteEvent, &rawEvents)
					if err != nil {
						log.Error.Println(err)
						continue
					}

					for _, event := range rawEvents {
						toSend = append(toSend, event)
					}

				}

				data, errl := json.Marshal(toSend)
				if errl != nil {
					log.Error.Println(errl)
					continue
				}
				before := time.Now()
				errp := eventsRecorder.RecordRaw("/events/bulk", data, dtos.Metadata{
					SDKVersion:  sdkVersion,
					MachineIP:   machineIP,
					MachineName: machineName,
				})
				if errp != nil {
					log.Error.Println(errp)
					interfaces.ProxyTelemetryWrapper.LocalTelemtry.IncCounter("backend::request.error")
				} else {
					bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
					interfaces.ProxyTelemetryWrapper.LocalTelemtry.IncLatency("backend::/api/events/bulk", bucket)
					interfaces.ProxyTelemetryWrapper.LocalTelemtry.IncCounter("backend::request.ok")
				}

			}
		}
	}
	// Clear the eventPoolBuffer
	eventPoolBuffer = make(sdkVersionBuffer)
}

// StopEventsRecording stops all tasks related to event submission.
func StopEventsRecording() {
	eventPoolBufferChannel <- eventChannelMessageStop
	for i := 0; i < eventChannelCapacity; i++ {
		eventWorkersStopChannel <- true
	}
}
