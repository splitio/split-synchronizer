package controllers

import (
	"net/http"
	"os"
)

const envSdkURLNamespace = "SPLITIO_SDK_URL"
const envEventsURLNamespace = "SPLITIO_EVENTS_URL"

// GetSdkStatus checks the status of the SDK Server
func GetSdkStatus() map[string]interface{} {
	sdkURL := os.Getenv(envSdkURLNamespace) + "/version"
	sdkStatus := make(map[string]interface{})
	resp, err := http.Get(sdkURL)
	if err != nil || (resp != nil && resp.StatusCode != 200) {
		sdkStatus["healthy"] = false
		sdkStatus["message"] = "Cannot reach SDK service"
	} else {
		sdkStatus["healthy"] = true
		sdkStatus["message"] = "SDK service working as expected"
	}
	return sdkStatus
}

// GetEventsStatus checks the status of the Events Server
func GetEventsStatus() map[string]interface{} {
	eventsURL := os.Getenv(envEventsURLNamespace) + "/version"
	eventsStatus := make(map[string]interface{})
	resp, err := http.Get(eventsURL)
	if err != nil || (resp != nil && resp.StatusCode != 200) {
		eventsStatus["healthy"] = false
		eventsStatus["message"] = "Cannot reach Events service"
	} else {
		eventsStatus["healthy"] = true
		eventsStatus["message"] = "Events service working as expected"
	}
	return eventsStatus
}
