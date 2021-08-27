package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/interfaces"
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
	conf.Initialize()
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	interfaces.Initialize()

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

		var eventsInPost []dtos.EventDTO
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

	e1 := dtos.EventDTO{
		Key:             "some_key",
		EventTypeID:     "some_event",
		TrafficTypeName: "some_traffic_type",
	}

	e2 := dtos.EventDTO{
		Key:             "another_key",
		EventTypeID:     "some_event",
		TrafficTypeName: "some_traffic_type",
	}

	events := []dtos.EventDTO{e1, e2}

	data, err := json.Marshal(events)
	if err != nil {
		t.Error(err)
		return
	}

	// Init Impressions controller.
	wg := &sync.WaitGroup{}
	InitializeEventWorkers(200, 2, wg)
	AddEvents(data, "test-1.0.0", "127.0.0.1", "SOME_MACHINE_NAME")

	// Lets async function post impressions
	time.Sleep(time.Duration(4) * time.Second)
}
