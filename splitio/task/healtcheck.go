package task

import (
	"sync"
	"time"

	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/util"
)

var healtcheck = make(chan string, 1)
var healthySince time.Time

// StopHealtcheck stops StopHealtcheck task sendding signal
func StopHealtcheck() {
	select {
	case healtcheck <- "STOP":
	default:
	}
}

func getSdkStatus(sdkClient api.Client) bool {
	_, err := sdkClient.Get("/version")
	if err != nil {
		log.Instance.Debug(err.Error())
		return false
	}
	return true
}

func getEventsStatus(eventsClient api.Client) bool {
	_, err := eventsClient.Get("/version")
	if err != nil {
		log.Instance.Debug(err.Error())
		return false
	}
	return true
}

// GetStorageStatus checks status for split storage
func GetStorageStatus(splitStorage storage.SplitStorage) bool {
	_, err := splitStorage.ChangeNumber()
	if err != nil {
		log.Instance.Debug(err.Error())
		return false
	}
	return true
}

// CheckEventsSdkStatus checks status for event and sdk
func CheckEventsSdkStatus(sdkClient api.Client, eventsClient api.Client) (bool, bool) {
	eventStatus := getEventsStatus(eventsClient)
	sdkStatus := getSdkStatus(sdkClient)
	if healthySince.IsZero() && eventStatus && sdkStatus {
		healthySince = time.Now()
	} else {
		if !sdkStatus || !eventStatus {
			healthySince = time.Time{}
		}
	}
	return eventStatus, sdkStatus
}

// CheckProducerStatus checks producer status
func CheckProducerStatus(splitStorage storage.SplitStorage, sdkClient api.Client, eventsClient api.Client) (bool, bool, bool) {
	eventStatus, sdkStatus := CheckEventsSdkStatus(sdkClient, eventsClient)
	storageStatus := GetStorageStatus(splitStorage)
	if healthySince.IsZero() && eventStatus && sdkStatus && storageStatus {
		healthySince = time.Now()
	} else {
		if !sdkStatus || !eventStatus || !storageStatus {
			healthySince = time.Time{}
		}
	}
	return eventStatus, sdkStatus, storageStatus
}

// CheckEnvirontmentStatus task to check status of Synchronizer
func CheckEnvirontmentStatus(wg *sync.WaitGroup, splitStorage storage.SplitStorage, sdkClient api.Client, eventsClient api.Client) {
	wg.Add(1)
	keepLoop := true
	for keepLoop {
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			CheckProducerStatus(splitStorage, sdkClient, eventsClient)
		} else {
			CheckEventsSdkStatus(sdkClient, eventsClient)
		}

		select {
		case msg := <-healtcheck:
			if msg == "STOP" {
				log.Instance.Debug("Stopping task: healtheck")
				keepLoop = false
			}
		case <-time.After(time.Duration(60) * time.Second):
		}
	}
	wg.Done()
}

// GetHealthySince returns last time that healtcheck was successful
func GetHealthySince() string {
	if healthySince.IsZero() {
		return "0"
	}
	return healthySince.Format("01-02-2006 15:04:05")
}

// GetHealthySinceTimestamp returns timestamp of the last healthceck that was ok
func GetHealthySinceTimestamp() string {
	if healthySince.IsZero() {
		return "0"
	}
	return util.ParseTime(healthySince)
}
