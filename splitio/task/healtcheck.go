package task

import (
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/storage"
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

func getSdkStatus() bool {
	_, err := api.SdkClient.Get("/version")
	if err != nil {
		log.Debug.Println(err.Error())
		return false
	}
	return true
}

func getEventsStatus() bool {
	_, err := api.EventsClient.Get("/version")
	if err != nil {
		log.Debug.Println(err.Error())
		return false
	}
	return true
}

// GetStorageStatus checks status for split storage
func GetStorageStatus(splitStorage storage.SplitStorage) bool {
	_, err := splitStorage.ChangeNumber()
	if err != nil {
		log.Debug.Println(err.Error())
		return false
	}
	return true
}

// CheckEventsSdkStatus checks status for event and sdk
func CheckEventsSdkStatus() (bool, bool) {
	eventStatus := getEventsStatus()
	sdkStatus := getSdkStatus()
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
func CheckProducerStatus(splitStorage storage.SplitStorage) (bool, bool, bool) {
	eventStatus, sdkStatus := CheckEventsSdkStatus()
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
func CheckEnvirontmentStatus(wg *sync.WaitGroup, splitStorage storage.SplitStorage) {
	wg.Add(1)
	keepLoop := true
	for keepLoop {
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			CheckProducerStatus(splitStorage)
		} else {
			CheckEventsSdkStatus()
		}

		select {
		case msg := <-healtcheck:
			if msg == "STOP" {
				log.Debug.Println("Stopping task: healtheck")
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
