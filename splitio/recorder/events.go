package recorder

import (
	"encoding/json"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

// EventsHTTPRecorder implrements EventsRecorder interface
type EventsHTTPRecorder struct{}

// Post send events to Split Events servers
func (r EventsHTTPRecorder) Post(events []api.EventDTO, sdkVersion string, machineIP string, machineName string) error {

	data, err := json.Marshal(events)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostEvents(data, sdkVersion, machineIP, machineName); err != nil {
		log.Error.Println("Error posting events", err.Error())
		return err
	}

	return nil
}
