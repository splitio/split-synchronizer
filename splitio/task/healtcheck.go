package task

import (
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
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

func getAuthStatus(authClient api.Client) bool {
	_, err := authClient.Get("/version", nil)
	if err != nil {
		log.Instance.Debug(err.Error())
		return false
	}
	return true
}

func getSdkStatus(sdkClient api.Client) bool {
	_, err := sdkClient.Get("/version", nil)
	if err != nil {
		log.Instance.Debug(err.Error())
		return false
	}
	return true
}

func getEventsStatus(eventsClient api.Client) bool {
	_, err := eventsClient.Get("/version", nil)
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

// CheckSplitServers checks status for splits servers
func CheckSplitServers(httpClients common.HTTPClients) (bool, bool, bool) {
	eventStatus := getEventsStatus(httpClients.EventsClient)
	sdkStatus := getSdkStatus(httpClients.SdkClient)
	authStatus := getAuthStatus(httpClients.AuthClient)
	if healthySince.IsZero() && eventStatus && sdkStatus && authStatus {
		healthySince = time.Now()
	} else {
		if !sdkStatus || !eventStatus || !authStatus {
			healthySince = time.Time{}
		}
	}
	return eventStatus, sdkStatus, authStatus
}

// CheckProducerStatus checks producer status
func CheckProducerStatus(splitStorage storage.SplitStorage, httpClients common.HTTPClients) (bool, bool, bool, bool) {
	eventStatus, sdkStatus, authStatus := CheckSplitServers(httpClients)
	storageStatus := GetStorageStatus(splitStorage)
	if healthySince.IsZero() && eventStatus && sdkStatus && storageStatus && authStatus {
		healthySince = time.Now()
	} else {
		if !sdkStatus || !eventStatus || !authStatus || !storageStatus {
			healthySince = time.Time{}
		}
	}
	return eventStatus, sdkStatus, authStatus, storageStatus
}

// CheckEnvirontmentStatus task to check status of Synchronizer
func CheckEnvirontmentStatus(wg *sync.WaitGroup, splitStorage storage.SplitStorage, httpClients common.HTTPClients) {
	wg.Add(1)
	keepLoop := true
	idleDuration := time.Duration(60) * time.Second
	timer := time.NewTimer(idleDuration)
	for keepLoop {
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			CheckProducerStatus(splitStorage, httpClients)
		} else {
			CheckSplitServers(httpClients)
		}

		timer.Reset(idleDuration)
		select {
		case msg := <-healtcheck:
			if msg == "STOP" {
				log.Instance.Debug("Stopping task: healtheck")
				keepLoop = false
			}
		case <-timer.C:
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
