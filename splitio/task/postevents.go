package task

import (
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

func taskPostEvents(tid int,
	recorderAdapter recorder.EventsRecorder,
	storageAdapter storage.EventStorage,
	bulkSize int64,
) {

	//[SDKVersion][MachineIP][MachineName]
	toSend := make(map[string]map[string]map[string][]api.EventDTO)

	storedEvents, err := storageAdapter.PopN(bulkSize)
	if err != nil {
		log.Error.Println("(Task) Post Events fails fetching events from storage", err.Error())
		return
	}

	for _, stored := range storedEvents {

		if stored.Metadata.SDKVersion == "" ||
			stored.Metadata.MachineIP == "" {
			continue
		}

		sdk := stored.Metadata.SDKVersion
		ip := stored.Metadata.MachineIP
		mname := stored.Metadata.MachineName

		if mname == "" {
			mname = "unknown"
		}

		if toSend[sdk] == nil {
			toSend[sdk] = make(map[string]map[string][]api.EventDTO)
		}

		if toSend[sdk][ip] == nil {
			toSend[sdk][ip] = make(map[string][]api.EventDTO)
		}

		if toSend[sdk][ip][mname] == nil {
			toSend[sdk][ip][mname] = make([]api.EventDTO, 0)
		}

		toSend[sdk][ip][mname] = append(toSend[sdk][ip][mname], stored.Event)
	}

	// TODO check to send data and posted to server
}

// PostEvents post events to Split Server task
func PostEvents(
	tid int,
	eventsRecorderAdapter recorder.EventsRecorder,
	eventsStorageAdapter storage.EventStorage,
	eventsRefreshRate int,
	eventsBulkSize int,
) {

	for {

		time.Sleep(time.Duration(eventsRefreshRate) * time.Second)
	}

}
