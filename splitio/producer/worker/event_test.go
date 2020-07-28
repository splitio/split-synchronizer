package worker

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service/api"
	recorderMock "github.com/splitio/go-split-commons/service/mocks"
	"github.com/splitio/go-split-commons/storage"
	storageMock "github.com/splitio/go-split-commons/storage/mocks"
	"github.com/splitio/go-toolkit/logging"
)

func TestSynhronizeEventError(t *testing.T) {
	eventMockStorage := storageMock.MockEventStorage{
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
		storage.NewMetricWrapper(storageMock.MockMetricStorage{}, nil, nil),
		logging.NewLogger(&logging.LoggerOptions{}),
	)

	err := eventSync.SynchronizeEvents(50)
	if err == nil {
		t.Error("It should return err")
	}
}

func TestSynhronizeEventWithNoEvents(t *testing.T) {
	eventMockStorage := storageMock.MockEventStorage{
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
		storage.NewMetricWrapper(storageMock.MockMetricStorage{}, nil, nil),
		logging.NewLogger(&logging.LoggerOptions{}),
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

	eventMockStorage := storageMock.MockEventStorage{
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
				if len(events) != 2 {
					t.Error("Wrong length of events passed")
				}
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

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventMockRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{
			IncCounterCall: func(key string) {
				if key != "events.status.200" {
					t.Error("Unexpected counter key to increase")
				}
			},
			IncLatencyCall: func(metricName string, index int) {
				if metricName != "events.time" {
					t.Error("Unexpected latency key to track")
				}
			},
		}, nil, nil),
		logging.NewLogger(&logging.LoggerOptions{}),
	)

	err := eventSync.SynchronizeEvents(50)
	if err != nil {
		t.Error("It should not return err")
	}
}

func TestSynhronizeEventSync(t *testing.T) {
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

	logger := logging.NewLogger(&logging.LoggerOptions{})
	eventRecorder := api.NewHTTPEventsRecorder(
		"",
		conf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		logger,
	)

	eventMockStorage := storageMock.MockEventStorage{
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

	eventSync := NewEventRecorderMultiple(
		eventMockStorage,
		eventRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{
			IncCounterCall: func(key string) {
				if key != "events.status.200" {
					t.Error("Unexpected counter key to increase")
				}
			},
			IncLatencyCall: func(metricName string, index int) {
				if metricName != "events.time" {
					t.Error("Unexpected latency key to track")
				}
			},
		}, nil, nil),
		logger,
	)

	eventSync.SynchronizeEvents(50)

	if requestReceived != 2 {
		t.Error("It should call twice")
	}
}
