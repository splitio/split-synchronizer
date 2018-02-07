package task

import (
	"errors"
	"math/rand"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

const totalPostAttemps = 3

func taskPostEvents(tid int,
	recorderAdapter recorder.EventsRecorder,
	storageAdapter storage.EventStorage,
	bulkSize int64,
) {

	//[SDKVersion][MachineIP][MachineName]
	toSend := make(map[string]map[string]map[string][]api.EventDTO)

	storedEvents, err := storageAdapter.PopN(bulkSize) //PopN has a mutex, so this function can be async without issues
	if err != nil {
		log.Error.Println("(Task) Post Events fails fetching events from storage", err.Error())
		return
	}

	for _, stored := range storedEvents {

		if stored.Metadata.SDKVersion == "" {
			continue
		}

		sdk := stored.Metadata.SDKVersion
		ip := stored.Metadata.MachineIP
		mname := stored.Metadata.MachineName

		if ip == "" {
			ip = "unknown"
		}

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

	for s, byIP := range toSend {
		for i, byName := range byIP {
			for n, bulk := range byName {

				var err = errors.New("") // forcing error to start "for" attempts
				attemps := 0
				for err != nil && attemps < totalPostAttemps {
					err = recorderAdapter.Post(bulk, s, i, n)
					if err != nil {
						log.Error.Println("Error posting events", err)
					}
					attemps++
					time.Sleep(time.Duration(rand.Intn(30)) * time.Second)
				}

			}
		}
	}
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
		taskPostEvents(tid, eventsRecorderAdapter, eventsStorageAdapter, int64(eventsBulkSize))
		time.Sleep(time.Duration(eventsRefreshRate) * time.Second)
	}

}
