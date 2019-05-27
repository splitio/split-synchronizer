package redis

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func makeEvents(key string, eventTypeID string, time int64, trafficTypeName string, value *float64, count int) []api.EventDTO {
	evts := make([]api.EventDTO, count)
	for i := 0; i < count; i++ {
		evts[i] = api.EventDTO{
			Key:             key,
			EventTypeID:     eventTypeID,
			Timestamp:       time + int64(i),
			TrafficTypeName: trafficTypeName,
			Value:           value,
		}
	}
	return evts
}
func TestEventsPOPN(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()
	Initialize(conf.Data.Redis)

	//Testing data
	keyPreffix := "splitsyncunittest"
	itemsToAdd := 50
	itemsToFetch := 10

	prefixAdapter := &prefixAdapter{prefix: keyPreffix}
	eventListName := prefixAdapter.eventsListNamespace()

	eventJSON := `{"m":{"s":"php-5.3.23","i":"192.168.232.255","n":"ip-192-168-232-255"},"e":{"key":"6c4829ab-a0d8-4e72-8176-a334f596fb79","trafficTypeName":"user","eventTypeId":"a5213963-5564-43ff-83b2-ac6dbd5af3b1","value":123.234234,"timestamp":1516310749882}}`

	//Deleting previous test data
	res := Client.Del(eventListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 50 events
	for i := 0; i < itemsToAdd; i++ {
		Client.RPush(eventListName, eventJSON)
	}

	adapter := NewEventStorageAdapter(Client, keyPreffix)
	// POPing first 10 events
	data, err := adapter.PopN(int64(itemsToFetch))
	if err != nil {
		t.Error(data)
		return
	}

	if len(data) != itemsToFetch {
		t.Error("Error list length, should be ", itemsToFetch, " and is ", len(data))
		return
	}

	llen := Client.LLen(eventListName)
	if llen.Err() != nil {
		t.Error(llen.Err().Error())
		return
	}

	if llen.Val() != 0 {
		t.Error("All elements should have been removed from redis and pushed into the in-memory cache")
	}

	if adapter.cache.Count() != itemsToAdd-itemsToFetch {
		t.Error("Error trimming the list in Redis")
		return
	}

	//Checking metadata
	if data[0].Metadata.SDKVersion != "php-5.3.23" {
		t.Error("Error reading metadata SDK version")
		return
	}

	if data[0].Metadata.MachineIP != "192.168.232.255" {
		t.Error("Error reading metadata machine IP")
		return
	}

	if data[0].Metadata.MachineName != "ip-192-168-232-255" {
		t.Error("Error reading metadata machine name")
		return
	}

	// Checking event data
	if data[0].Event.Key != "6c4829ab-a0d8-4e72-8176-a334f596fb79" {
		t.Error("Error reading event key")
		return
	}

	if data[0].Event.EventTypeID != "a5213963-5564-43ff-83b2-ac6dbd5af3b1" {
		t.Error("Error reading event eventTypeID")
		return
	}

	if *data[0].Event.Value != 123.234234 {
		t.Error("Error reading event value")
		return
	}

	if data[0].Event.TrafficTypeName != "user" {
		t.Error("Error reading event trafficTypeName")
		return
	}

	if data[0].Event.Timestamp != 1516310749882 {
		t.Error("Error reading event timestamp")
		return
	}

	adapter.client.Del("splitsyncunittest.SPLITIO.events")
}

func TestEventsSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.eventsListNamespace())

	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	eventsRaw := makeEvents("key", "eventTypeId", 123456, "trafficTypeName", nil, 30)

	//Adding events to retrieve.
	for _, event := range eventsRaw {
		toStore, err := json.Marshal(api.RedisStoredEventDTO{
			Event: api.EventDTO{
				Key:             event.Key,
				EventTypeID:     event.EventTypeID,
				Timestamp:       event.Timestamp,
				TrafficTypeName: event.TrafficTypeName,
				Value:           event.Value,
			},
			Metadata: api.RedisStoredMachineMetadataDTO{
				MachineIP:   metadata.MachineIP,
				MachineName: metadata.MachineName,
				SDKVersion:  metadata.SdkVersion,
			},
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		Client.RPush(
			prefixAdapter.eventsListNamespace(),
			toStore,
		)
	}
	eventsStorageAdapter := NewEventStorageAdapter(Client, "")
	size := eventsStorageAdapter.Size()
	if size != 30 {
		t.Error("Size is not the expected one. Expected 200. Actual", size)
	}
	Client.Del(prefixAdapter.eventsListNamespace())
}
