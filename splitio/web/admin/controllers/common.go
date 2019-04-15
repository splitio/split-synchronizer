package controllers

import (
	"github.com/splitio/split-synchronizer/splitio/api"

	"github.com/splitio/split-synchronizer/log"
)

const envSdkURLNamespace = "SPLITIO_SDK_URL"
const envEventsURLNamespace = "SPLITIO_EVENTS_URL"

// GetSdkStatus checks the status of the SDK Server
func GetSdkStatus() map[string]interface{} {
	_, err := api.SdkClient.Get("/version")
	sdkStatus := make(map[string]interface{})
	if err != nil {
		sdkStatus["healthy"] = false
		sdkStatus["message"] = "Cannot reach SDK service"
		log.Debug.Println("Events Server:", err)
	} else {
		sdkStatus["healthy"] = true
		sdkStatus["message"] = "SDK service working as expected"
	}
	return sdkStatus
}

// GetEventsStatus checks the status of the Events Server
func GetEventsStatus() map[string]interface{} {
	_, err := api.EventsClient.Get("/version")
	eventsStatus := make(map[string]interface{})
	if err != nil {
		eventsStatus["healthy"] = false
		eventsStatus["message"] = "Cannot reach Events service"
		log.Debug.Println("Events Server:", err)
	} else {
		eventsStatus["healthy"] = true
		eventsStatus["message"] = "Events service working as expected"
	}
	return eventsStatus
}
