package task

import (
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

var healtcheck = make(chan string, 1)

// StopHealtcheck stops StopHealtcheck task sendding signal
func StopHealtcheck() {
	select {
	case healtcheck <- "STOP":
	default:
	}
}

var healthySince time.Time

// GetSdkStatus checks the status of the SDK Server
func GetSdkStatus() bool {
	_, err := api.SdkClient.Get("/version")
	if err != nil {
		log.Debug.Println(err.Error())
		return false
	}
	return true
}

// GetEventsStatus checks the status of the Events Server
func GetEventsStatus() bool {
	_, err := api.EventsClient.Get("/version")
	if err != nil {
		log.Debug.Println(err.Error())
		return false
	}
	return true
}

// GetStorageStatus checks the status of the Storage
func GetStorageStatus(splitStorage storage.SplitStorage) bool {
	if appcontext.ExecutionMode() == appcontext.ProducerMode {
		_, err := splitStorage.ChangeNumber()
		if err != nil {
			log.Debug.Println(err.Error())
			return false
		}
	}
	return true
}

func taskCheckEnvirontmentStatus(splitStorage storage.SplitStorage) {
	sdkStatus := GetSdkStatus()
	eventsStatus := GetEventsStatus()
	storageStatus := GetStorageStatus(splitStorage)

	if sdkStatus && eventsStatus && storageStatus {
		healthySince = time.Now()
	}
}

// CheckEnvirontmentStatus task to check status of Synchronizer
func CheckEnvirontmentStatus(wg *sync.WaitGroup, splitStorage storage.SplitStorage) {
	wg.Add(1)
	keepLoop := true
	for keepLoop {
		taskCheckEnvirontmentStatus(splitStorage)

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
		return "---"
	}
	return healthySince.Format("01-02-2006 15:04:05")
}

// GetHealthySinceTimestamp returns timestamp of the last healthceck that was ok
func GetHealthySinceTimestamp() string {
	if healthySince.IsZero() {
		return "- -"
	}
	subs := (time.Now()).Sub(healthySince)
	return time.Time{}.Add(subs).Format("15:04:05")
}
