package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

func TestTaskPostEvents(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	bulkSize := 10

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")
		sdkMachineName := r.Header.Get("SplitSDKMachineName")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		if sdkMachineName != "SOME_MACHINE_NAME" {
			t.Error("SDK Machine Name HEADER not match")
		}

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if len(eventsInPost) != bulkSize {
			t.Error("Invalid amount of events")
		}

		if eventsInPost[0].EventTypeID != "a5213963-5564-43ff-83b2-ac6dbd5af3b1" {
			t.Error("Invalid EventTypeID")
		}

		if eventsInPost[0].Key != "6c4829ab-a0d8-4e72-8176-a334f596fb79" {
			t.Error("Invalid KEY")
		}

		if eventsInPost[0].Timestamp != 1516310749882 {
			t.Error("Invalid Timestamp")
		}

		if eventsInPost[0].TrafficTypeName != "user" {
			t.Error("Invalid TrafficTypeName")
		}

		if *eventsInPost[0].Value != 2993.4876 {
			t.Error("Invalid Value")
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "posteventunittest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 50
	eventListName := conf.Data.Redis.Prefix + ".SPLITIO.events"

	eventJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"e":{"key":"6c4829ab-a0d8-4e72-8176-a334f596fb79","trafficTypeName":"user","eventTypeId":"a5213963-5564-43ff-83b2-ac6dbd5af3b1","value":2993.4876,"timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(eventListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 50 events
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(eventListName, eventJSON)
	}
	//----------------

	tid := 1
	eventsRecorderAdapter := recorder.EventsHTTPRecorder{}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskPostEvents(tid, eventsRecorderAdapter, eventsStorageAdapter, int64(bulkSize))
	}()

	time.Sleep(10 * time.Second)
}

func TestFlushEvents(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	size := 4

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "posteventunittest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 5
	eventListName := conf.Data.Redis.Prefix + ".SPLITIO.events"

	eventJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"e":{"key":"6c4829ab-a0d8-4e72-8176-a334f596fb79","trafficTypeName":"user","eventTypeId":"a5213963-5564-43ff-83b2-ac6dbd5af3b1","value":2993.4876,"timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(eventListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 5 events
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(eventListName, eventJSON)
	}
	//----------------

	eventsRecorderAdapter := recorder.EventsHTTPRecorder{}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		count := int64(size)
		EventsFlush(eventsRecorderAdapter, eventsStorageAdapter, &count)
		total := eventsStorageAdapter.Size()
		if total != 1 {
			t.Error("It should kept 1 element")
		}
	}()
}

func TestFlushEventsInBatches(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	size := 10001

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "posteventunittest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 10003
	eventListName := conf.Data.Redis.Prefix + ".SPLITIO.events"

	eventJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"e":{"key":"6c4829ab-a0d8-4e72-8176-a334f596fb79","trafficTypeName":"user","eventTypeId":"a5213963-5564-43ff-83b2-ac6dbd5af3b1","value":2993.4876,"timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(eventListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 10003 events
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(eventListName, eventJSON)
	}
	//----------------

	eventsRecorderAdapter := recorder.EventsHTTPRecorder{}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		count := int64(size)
		EventsFlush(eventsRecorderAdapter, eventsStorageAdapter, &count)
		total := eventsStorageAdapter.Size()
		if total != 2 {
			t.Error("It should kept 2 element, but there are:", total)
		}
	}()
}

func TestFlushEventsNilSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "posteventunittest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 50001
	eventListName := conf.Data.Redis.Prefix + ".SPLITIO.events"

	eventJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"e":{"key":"6c4829ab-a0d8-4e72-8176-a334f596fb79","trafficTypeName":"user","eventTypeId":"a5213963-5564-43ff-83b2-ac6dbd5af3b1","value":2993.4876,"timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(eventListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 50001 events
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(eventListName, eventJSON)
	}
	//----------------

	eventsRecorderAdapter := recorder.EventsHTTPRecorder{}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()

		EventsFlush(eventsRecorderAdapter, eventsStorageAdapter, nil)
		total := eventsStorageAdapter.Size()
		if total != 25001 {
			t.Error("It should evict 25000 elements. The remaining elements are:", total)
		}
	}()

	redis.Client.Del("posteventunittest.SPLITIO.events")
}
