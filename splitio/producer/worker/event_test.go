package worker

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service/api"
	recorderMock "github.com/splitio/go-split-commons/v4/service/mocks"
	storageMock "github.com/splitio/go-split-commons/v4/storage/mocks"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"
	evCalcMock "github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc/mocks"
)

func TestEventWorkerStorageError(t *testing.T) {
	logger := logging.NewLogger(nil)
	eventMockStorage := storageMock.MockEventStorage{
		CountCall: func() int64 { return 0 },
		PopNWithMetadataCall: func(n int64) ([]dtos.QueueStoredEventDTO, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return make([]dtos.QueueStoredEventDTO, 0), errors.New("Some")
		},
	}

	eventMockRecorder := recorderMock.MockEventRecorder{}

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventMockRecorder,
		&storageMock.MockTelemetryStorage{},
		&evCalcMock.EvCalcMock{
			StoreDataFlushedCall: func(_ time.Time, _ int, _ int64) { t.Error("StoreDataFlushedCall should not be called") },
			AcquireCall:          func() bool { t.Error("Aquire should not be called"); return false },
			ReleaseCall:          func() { t.Error("Release should not be called") },
			BusyCall:             func() bool { return false },
		},
		logger,
	)

	err := eventSync.SynchronizeEvents(50)
	if err == nil {
		t.Error("It should return err")
	}
}

func TestSynhronizeEventWithNoEvents(t *testing.T) {
	logger := logging.NewLogger(nil)
	eventMockStorage := storageMock.MockEventStorage{
		CountCall: func() int64 { return 0 }, // TODO: Check!
		PopNWithMetadataCall: func(n int64) ([]dtos.QueueStoredEventDTO, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return make([]dtos.QueueStoredEventDTO, 0), nil
		},
	}

	eventMockRecorder := recorderMock.MockEventRecorder{
		RecordCall: func(events []dtos.EventDTO, metadata dtos.Metadata) error {
			t.Error("It should not be called")
			return nil
		},
	}

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventMockRecorder,
		&storageMock.MockTelemetryStorage{},
		&evCalcMock.EvCalcMock{
			StoreDataFlushedCall: func(_ time.Time, _ int, _ int64) { t.Error("StoreDataFlushedCall should not be called") },
			AcquireCall:          func() bool { t.Error("Aquire should not be called"); return false },
			ReleaseCall:          func() { t.Error("Release should not be called") },
			BusyCall:             func() bool { return false },
		},
		logger,
	)

	err := eventSync.SynchronizeEvents(50)
	if err != nil {
		t.Error("It should not return err")
	}
}

func wrapEvent(key string) dtos.EventDTO {
	return dtos.EventDTO{
		EventTypeID:     "someId",
		Key:             key,
		Properties:      make(map[string]interface{}),
		Timestamp:       123456789,
		TrafficTypeName: "someTraffic",
		Value:           nil,
	}
}

func TestSynhronizeEvent(t *testing.T) {
	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{MachineIP: "1.1.1.1", MachineName: "machine1", SDKVersion: "go-1.1.1"}
	metadata2 := dtos.Metadata{MachineIP: "2.2.2.2", MachineName: "machine2", SDKVersion: "php-2.2.2"}

	eventMockStorage := storageMock.MockEventStorage{
		CountCall: func() int64 { return 0 }, // TODO: Check!
		PopNWithMetadataCall: func(n int64) ([]dtos.QueueStoredEventDTO, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.QueueStoredEventDTO{
				{Event: wrapEvent("key1"), Metadata: metadata1},
				{Event: wrapEvent("key2"), Metadata: metadata1},
				{Event: wrapEvent("key3"), Metadata: metadata2},
				{Event: wrapEvent("key4"), Metadata: metadata2},
				{Event: wrapEvent("key5"), Metadata: metadata1},
			}, nil
		},
	}

	eventMockRecorder := recorderMock.MockEventRecorder{
		RecordCall: func(events []dtos.EventDTO, metadata dtos.Metadata) error {
			switch len(events) {
			case 3:
				if events[0].Key != "key1" {
					t.Error("Wrong event received")
				}
				if events[1].Key != "key2" {
					t.Error("Wrong event received")
				}
				if events[2].Key != "key5" {
					t.Error("Wrong event received")
				}
				if metadata.SDKVersion != "go-1.1.1" {
					t.Error("Wrong metadata")
				}
			case 2:
				if events[0].Key != "key3" {
					t.Error("Wrong event received")
				}
				if events[1].Key != "key4" {
					t.Error("Wrong event received")
				}
				if metadata.SDKVersion != "php-2.2.2" {
					t.Error("Wrong metadata")
				}
			default:
				t.Error("Unexpected case")
			}

			return nil
		},
	}

	type sfcall struct {
		flushed   int
		remaining int64
	}
	calls := []sfcall{}
	evCalc := evCalcMock.EvCalcMock{
		StoreDataFlushedCall: func(_ time.Time, flushed int, remaining int64) { calls = append(calls, sfcall{flushed, remaining}) },
		AcquireCall:          func() bool { t.Error("Aquire should not have been called."); return false },
		ReleaseCall:          func() { t.Error("Release should not have been called") },
		BusyCall:             func() bool { return false },
	}

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventMockRecorder,
		&storageMock.MockTelemetryStorage{
			RecordSyncLatencyCall: func(resource int, latency time.Duration) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
			RecordSuccessfulSyncCall: func(resource int, when time.Time) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
		},
		&evCalc,
		logger,
	)

	err := eventSync.SynchronizeEvents(50)
	if err != nil {
		t.Error("It should not return err")
	}

	// Since the data is originally held in a map and the ordering of keys is undefined,
	// we sort them in a slice prior to checking them
	sort.Slice(calls, func(i, j int) bool { return calls[i].flushed < calls[j].flushed })
	if calls[0].flushed != 2 && calls[0].remaining != 3 {
		t.Error("should have flushed 2 and left 3.")
	}

	if calls[1].flushed != 3 && calls[1].remaining != 0 {
		t.Error("should have flushed 3 and left 0.")
	}
}

func TestSynhronizeEventE2E(t *testing.T) {
	logger := logging.NewLogger(nil)
	var requestReceived int64

	metadata1 := dtos.Metadata{
		MachineIP:   "1.1.1.1",
		MachineName: "machine1",
		SDKVersion:  "go-1.1.1",
	}
	metadata2 := dtos.Metadata{
		MachineIP:   "2.2.2.2",
		MachineName: "machine2",
		SDKVersion:  "php-2.2.2",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" && r.Method != "POST" {
			t.Error("Invalid request. Should be POST to /events")
		}
		atomic.AddInt64(&requestReceived, 1)

		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Error("Error reading body")
			return
		}

		var events []dtos.EventDTO

		err = json.Unmarshal(body, &events)
		if err != nil {
			t.Errorf("Error parsing json: %s", err)
			return
		}

		switch len(events) {
		case 3:
			if r.Header.Get("SplitSDKVersion") != "go-1.1.1" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineName") != "machine1" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineIP") != "1.1.1.1" {
				t.Error("Unexpected version in header")
			}
			if len(events) != 3 {
				t.Error("Incorrect number of events")
			}
		case 2:
			if r.Header.Get("SplitSDKVersion") != "php-2.2.2" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineName") != "machine2" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineIP") != "2.2.2.2" {
				t.Error("Unexpected version in header")
			}
			if len(events) != 2 {
				t.Error("Incorrect number of events")
			}
		default:
			t.Error("Unexpected case")
		}

		return
	}))
	defer ts.Close()

	eventRecorder := api.NewHTTPEventsRecorder(
		"",
		conf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
	)

	eventMockStorage := storageMock.MockEventStorage{
		CountCall: func() int64 { return 0 }, // TODO: Check!
		PopNWithMetadataCall: func(n int64) ([]dtos.QueueStoredEventDTO, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.QueueStoredEventDTO{
				{Event: wrapEvent("key1"), Metadata: metadata1},
				{Event: wrapEvent("key2"), Metadata: metadata1},
				{Event: wrapEvent("key3"), Metadata: metadata2},
				{Event: wrapEvent("key4"), Metadata: metadata2},
				{Event: wrapEvent("key5"), Metadata: metadata1},
			}, nil
		},
	}

	type sfcall struct {
		flushed   int
		remaining int64
	}
	calls := []sfcall{}
	evCalc := evCalcMock.EvCalcMock{
		StoreDataFlushedCall: func(_ time.Time, flushed int, remaining int64) { calls = append(calls, sfcall{flushed, remaining}) },
		AcquireCall:          func() bool { t.Error("Aquire should not have been called."); return false },
		ReleaseCall:          func() { t.Error("Release should not have been called") },
		BusyCall:             func() bool { return false },
	}

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventRecorder,
		&storageMock.MockTelemetryStorage{
			RecordSyncLatencyCall: func(resource int, latency time.Duration) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
			RecordSuccessfulSyncCall: func(resource int, when time.Time) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
		},
		&evCalc,
		logger,
	)

	eventSync.SynchronizeEvents(50)

	if requestReceived != 2 {
		t.Error("It should call twice")
	}

	sort.Slice(calls, func(i, j int) bool { return calls[i].flushed < calls[j].flushed })
	if calls[0].flushed != 2 && calls[0].remaining != 3 {
		t.Error("should have flushed 2 and left 3.")
	}

	if calls[1].flushed != 3 && calls[1].remaining != 0 {
		t.Error("should have flushed 3 and left 0.")
	}

}

func TestFlushEvents(t *testing.T) {
	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{MachineIP: "1.1.1.1", MachineName: "machine1", SDKVersion: "go-1.1.1"}
	metadata2 := dtos.Metadata{MachineIP: "2.2.2.2", MachineName: "machine2", SDKVersion: "php-2.2.2"}

	eventMockStorage := storageMock.MockEventStorage{
		CountCall: func() int64 { return 5 },
		PopNWithMetadataCall: func(n int64) ([]dtos.QueueStoredEventDTO, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.QueueStoredEventDTO{
				{Event: wrapEvent("key1"), Metadata: metadata1},
				{Event: wrapEvent("key2"), Metadata: metadata1},
				{Event: wrapEvent("key3"), Metadata: metadata2},
				{Event: wrapEvent("key4"), Metadata: metadata2},
				{Event: wrapEvent("key5"), Metadata: metadata1},
			}, nil
		},
	}

	eventMockRecorder := recorderMock.MockEventRecorder{
		RecordCall: func(events []dtos.EventDTO, metadata dtos.Metadata) error {
			switch len(events) {
			case 3:
				if events[0].Key != "key1" {
					t.Error("Wrong event received")
				}
				if events[1].Key != "key2" {
					t.Error("Wrong event received")
				}
				if events[2].Key != "key5" {
					t.Error("Wrong event received")
				}
				if metadata.SDKVersion != "go-1.1.1" {
					t.Error("Wrong metadata")
				}
			case 2:
				if events[0].Key != "key3" {
					t.Error("Wrong event received")
				}
				if events[1].Key != "key4" {
					t.Error("Wrong event received")
				}
				if metadata.SDKVersion != "php-2.2.2" {
					t.Error("Wrong metadata")
				}
			default:
				t.Error("Unexpected case")
			}

			return nil
		},
	}

	type sfcall struct {
		flushed   int
		remaining int64
	}
	calls := []sfcall{}
	acquireCalls := 0
	releaseCalls := 0
	evCalc := evCalcMock.EvCalcMock{
		StoreDataFlushedCall: func(_ time.Time, flushed int, remaining int64) { calls = append(calls, sfcall{flushed, remaining}) },
		AcquireCall:          func() bool { acquireCalls++; return true },
		ReleaseCall:          func() { releaseCalls++ },
	}

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventMockRecorder,
		&storageMock.MockTelemetryStorage{
			RecordSyncLatencyCall: func(resource int, latency time.Duration) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
			RecordSuccessfulSyncCall: func(resource int, when time.Time) {
				if resource != telemetry.EventSync {
					t.Error("wrong resource")
				}
			},
		},
		&evCalc,
		logger,
	)

	err := eventSync.FlushEvents(50)
	if err != nil {
		t.Error("It should not return err")
	}

	// Since the data is originally held in a map and the ordering of keys is undefined,
	// we sort them in a slice prior to checking them
	sort.Slice(calls, func(i, j int) bool { return calls[i].flushed < calls[j].flushed })
	if calls[0].flushed != 2 && calls[0].remaining != 3 {
		t.Error("should have flushed 2 and left 3.")
	}

	if calls[1].flushed != 3 && calls[1].remaining != 0 {
		t.Error("should have flushed 3 and left 0.")
	}

	if acquireCalls != 1 || releaseCalls != 1 {
		t.Error("acquire & release should have been called once")
	}
}
