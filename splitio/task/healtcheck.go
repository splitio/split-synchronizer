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

var lastSucceed time.Time

func getSdkStatus() bool {
	_, err := api.SdkClient.Get("/version")
	if err != nil {
		return false
	}
	return true
}

func getEventsStatus() bool {
	_, err := api.EventsClient.Get("/version")
	if err != nil {
		return false
	}
	return true
}

func getStorageStatus(splitStorage storage.SplitStorage) bool {
	if appcontext.ExecutionMode() == appcontext.ProducerMode {
		_, err := splitStorage.ChangeNumber()
		if err != nil {
			return false
		}
	}
	return true
}

func taskCheckEnvirontmentStatus(splitStorage storage.SplitStorage) {
	sdkStatus := getSdkStatus()
	eventsStatus := getEventsStatus()
	storageStatus := getStorageStatus(splitStorage)

	if sdkStatus && eventsStatus && storageStatus {
		lastSucceed = time.Now()
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

// GetLastSucceed returns last time that healthech was succesful
func GetLastSucceed() string {
	if lastSucceed.IsZero() {
		return ""
	}
	return lastSucceed.Format("01-02-2006 15:04:05")
}
