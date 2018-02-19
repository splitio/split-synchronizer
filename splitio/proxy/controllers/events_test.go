package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func TestEventBufferCounter(t *testing.T) {
	var p = eventPoolBufferSizeStruct{size: 0}

	p.Addition(1)
	p.Addition(2)
	if !p.GreaterThan(2) || p.GreaterThan(4) {
		t.Error("Error on Addition method")
	}

	p.Reset()
	if !p.GreaterThan(-1) || p.GreaterThan(1) {
		t.Error("Error on Reset")
	}

}

func TestAddEvents(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "test-1.0.0" {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "127.0.0.1" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "SOME_MACHINE_NAME" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if eventsInPost[0].Key != "some_key" ||
			eventsInPost[0].EventTypeID != "some_event" ||
			eventsInPost[0].TrafficTypeName != "some_traffic_type" {
			t.Error("Posted events arrived mal-formed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	e1 := api.EventDTO{
		Key:             "some_key",
		EventTypeID:     "some_event",
		TrafficTypeName: "some_traffic_type",
	}

	e2 := api.EventDTO{
		Key:             "another_key",
		EventTypeID:     "some_event",
		TrafficTypeName: "some_traffic_type",
	}

	events := []api.EventDTO{e1, e2}

	data, err := json.Marshal(events)
	if err != nil {
		t.Error(err)
		return
	}

	// Init Impressions controller.
	InitializeEventWorkers(200, 2)
	AddEvents(data, "test-1.0.0", "127.0.0.1", "SOME_MACHINE_NAME")

	// Lets async function post impressions
	time.Sleep(time.Duration(4) * time.Second)
}
