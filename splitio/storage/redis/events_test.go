package redis

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

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
		t.Error("Error list length")
		return
	}

	llen := Client.LLen(eventListName)
	if llen.Err() != nil {
		t.Error(llen.Err().Error())
		return
	}

	if llen.Val() != int64(itemsToAdd-itemsToFetch) {
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

}
