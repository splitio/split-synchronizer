package controllers

import (
	"net/http"

	"github.com/splitio/split-synchronizer/log"
)

const sdkURL = "https://sdk.split.io/api/version"
const eventsURL = "https://events.split.io/api"

// GetSdkStatus checks the status of the SDK Server
func GetSdkStatus() map[string]interface{} {
	sdkStatus := make(map[string]interface{})
	resp, err := http.Get(sdkURL)
	if err != nil && resp.StatusCode != 200 {
		log.Error.Println("Error requesting data to API: ", sdkURL, err.Error())
		sdkStatus["healthy"] = false
		sdkStatus["message"] = err.Error()
	} else {
		sdkStatus["healthy"] = true
		sdkStatus["message"] = "SDK service working as expected"
	}
	return sdkStatus
}

// GetEventsStatus checks the status of the Events Server
func GetEventsStatus() map[string]interface{} {
	eventsStatus := make(map[string]interface{})
	respEvents, err := http.Get(eventsURL)
	if err != nil && respEvents.StatusCode != 200 {
		log.Error.Println("Error requesting data to API: ", eventsURL, err.Error())
		eventsStatus["healthy"] = false
		eventsStatus["message"] = err.Error()
	} else {
		eventsStatus["healthy"] = true
		eventsStatus["message"] = "Events service working as expected"
	}
	return eventsStatus
}
