package worker

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v3/conf"
	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-split-commons/v3/provisional"
	"github.com/splitio/go-split-commons/v3/service/api"
	recorderMock "github.com/splitio/go-split-commons/v3/service/mocks"
	"github.com/splitio/go-split-commons/v3/storage"
	storageMock "github.com/splitio/go-split-commons/v3/storage/mocks"
	"github.com/splitio/go-toolkit/v4/logging"
	"github.com/splitio/split-synchronizer/v4/log"
)

func TestSynchronizeImpressionError(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	impressionMockStorage := storageMock.MockImpressionStorage{
		PopNWithMetadataCall: func(n int64) ([]dtos.ImpressionQueueObject, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return make([]dtos.ImpressionQueueObject, 0), errors.New("Some")
		},
	}

	impressionMockRecorder := recorderMock.MockImpressionRecorder{}

	impressionSync, _ := NewImpressionRecordMultiple(
		impressionMockStorage,
		impressionMockRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{}, nil, nil),
		log.Instance,
		conf.ManagerConfig{
			ImpressionsMode: conf.ImpressionsModeDebug,
			OperationMode:   conf.ProducerSync,
			ListenerEnabled: true,
		},
		nil,
	)

	err := impressionSync.SynchronizeImpressions(50)
	if err == nil {
		t.Error("It should return err")
	}
}

func TestSynhronizeImpressionWithNoImpressions(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	impressionMockStorage := storageMock.MockImpressionStorage{
		PopNWithMetadataCall: func(n int64) ([]dtos.ImpressionQueueObject, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return make([]dtos.ImpressionQueueObject, 0), nil
		},
	}

	impressionMockRecorder := recorderMock.MockImpressionRecorder{
		RecordCall: func(impressions []dtos.ImpressionsDTO, metadata dtos.Metadata, extraHeaders map[string]string) error {
			t.Error("It should not be called")
			return nil
		},
	}

	impressionSync, _ := NewImpressionRecordMultiple(
		impressionMockStorage,
		impressionMockRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{}, nil, nil),
		log.Instance,
		conf.ManagerConfig{
			ImpressionsMode: conf.ImpressionsModeDebug,
			OperationMode:   conf.ProducerSync,
			ListenerEnabled: true,
		},
		nil,
	)

	err := impressionSync.SynchronizeImpressions(50)
	if err != nil {
		t.Error("It should not return err")
	}
}

func wrapImpression(feature string) dtos.Impression {
	return dtos.Impression{
		BucketingKey: "someBucketingKey",
		ChangeNumber: 123456789,
		KeyName:      "someKey",
		Label:        "someLabel",
		Time:         time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Treatment:    "someTreatment",
		FeatureName:  feature,
	}
}

func TestSynhronizeImpressionSyncDebug(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
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
		if r.URL.Path != "/impressions" && r.Method != "POST" {
			t.Error("Invalid request. Should be POST to /impressions")
		}
		atomic.AddInt64(&requestReceived, 1)

		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Error("Error reading body")
			return
		}

		var impressions []dtos.ImpressionsDTO

		err = json.Unmarshal(body, &impressions)
		if err != nil {
			t.Errorf("Error parsing json: %s", err)
			return
		}

		switch impressions[0].TestName {
		case "feature1":
			if r.Header.Get("SplitSDKVersion") != "go-1.1.1" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineName") != "machine1" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineIP") != "1.1.1.1" {
				t.Error("Unexpected version in header")
			}
			if len(impressions[0].KeyImpressions) != 3 {
				t.Error("Incorrect number of impressions")
			}
		case "feature2":
			if r.Header.Get("SplitSDKVersion") != "php-2.2.2" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineName") != "machine2" {
				t.Error("Unexpected version in header")
			}
			if r.Header.Get("SplitSDKMachineIP") != "2.2.2.2" {
				t.Error("Unexpected version in header")
			}
			if len(impressions[0].KeyImpressions) != 2 {
				t.Error("Incorrect number of impressions")
			}
		default:
			t.Error("Unexpected case")
		}
		return
	}))
	defer ts.Close()

	impressionRecorder := api.NewHTTPImpressionRecorder(
		"",
		conf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		log.Instance,
	)

	impressionMockStorage := storageMock.MockImpressionStorage{
		PopNWithMetadataCall: func(n int64) ([]dtos.ImpressionQueueObject, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.ImpressionQueueObject{
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
				{Impression: wrapImpression("feature2"), Metadata: metadata2},
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
				{Impression: wrapImpression("feature2"), Metadata: metadata2},
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
			}, nil
		},
	}

	impressionSync, _ := NewImpressionRecordMultiple(
		impressionMockStorage,
		impressionRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{
			IncCounterCall: func(key string) {
				if key != "testImpressions.status.200" {
					t.Error("Unexpected counter key to increase")
				}
			},
			IncLatencyCall: func(metricName string, index int) {
				if metricName != "testImpressions.time" {
					t.Error("Unexpected latency key to track")
				}
			},
		}, nil, nil),
		log.Instance,
		conf.ManagerConfig{
			ImpressionsMode: conf.ImpressionsModeDebug,
			OperationMode:   conf.ProducerSync,
			ListenerEnabled: true,
		},
		nil,
	)

	impressionSync.SynchronizeImpressions(50)

	if requestReceived != 2 {
		t.Error("It should call twice")
	}
}

func TestSynhronizeImpressionSyncOptimized(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
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
		if r.URL.Path != "/impressions" && r.Method != "POST" {
			t.Error("Invalid request. Should be POST to /impressions")
		}
		atomic.AddInt64(&requestReceived, 1)

		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Error("Error reading body")
			return
		}

		var impressions []dtos.ImpressionsDTO

		err = json.Unmarshal(body, &impressions)
		if err != nil {
			t.Errorf("Error parsing json: %s", err)
			return
		}

		switch impressions[0].TestName {
		case "feature1":
			if len(impressions[0].KeyImpressions) != 1 {
				t.Error("Incorrect number of impressions")
			}
		case "feature2":
			if len(impressions[0].KeyImpressions) != 1 {
				t.Error("Incorrect number of impressions")
			}
		default:
			t.Error("Unexpected case")
		}
		return
	}))
	defer ts.Close()

	impressionRecorder := api.NewHTTPImpressionRecorder(
		"",
		conf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		log.Instance,
	)

	impressionMockStorage := storageMock.MockImpressionStorage{
		PopNWithMetadataCall: func(n int64) ([]dtos.ImpressionQueueObject, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.ImpressionQueueObject{
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
				{Impression: wrapImpression("feature2"), Metadata: metadata2},
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
				{Impression: wrapImpression("feature2"), Metadata: metadata2},
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
			}, nil
		},
	}

	impressionSync, _ := NewImpressionRecordMultiple(
		impressionMockStorage,
		impressionRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{
			IncCounterCall: func(key string) {},
			IncLatencyCall: func(metricName string, index int) {},
		}, nil, nil),
		log.Instance,
		conf.ManagerConfig{
			ImpressionsMode: conf.ImpressionsModeOptimized,
			OperationMode:   conf.ProducerSync,
			ListenerEnabled: true,
		},
		provisional.NewImpressionsCounter(),
	)

	impressionSync.SynchronizeImpressions(50)

	if requestReceived != 2 {
		t.Error("It should be called twice")
	}
}

func TestSynhronizeImpressionPt(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	var requestReceived int64
	var pt int64

	metadata1 := dtos.Metadata{
		MachineIP:   "1.1.1.1",
		MachineName: "machine1",
		SDKVersion:  "go-1.1.1",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/impressions" && r.Method != "POST" {
			t.Error("Invalid request. Should be POST to /impressions")
		}
		atomic.AddInt64(&requestReceived, 1)

		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			t.Error("Error reading body")
			return
		}

		var impressions []dtos.ImpressionsDTO

		err = json.Unmarshal(body, &impressions)
		if err != nil {
			t.Errorf("Error parsing json: %s", err)
			return
		}

		if len(impressions[0].KeyImpressions) != 1 {
			t.Error("Incorrect number of impressions")
		}
		if atomic.LoadInt64(&requestReceived) == 1 {
			if impressions[0].KeyImpressions[0].Pt != 0 {
				t.Error("Unexpected pt")
			}
			atomic.StoreInt64(&pt, impressions[0].KeyImpressions[0].Time)
		}
		if atomic.LoadInt64(&requestReceived) == 2 {
			if impressions[0].KeyImpressions[0].Pt != atomic.LoadInt64(&pt) {
				t.Error("Unexpected pt")
			}
		}
		return
	}))
	defer ts.Close()

	impressionRecorder := api.NewHTTPImpressionRecorder(
		"",
		conf.AdvancedConfig{
			EventsURL: ts.URL,
			SdkURL:    ts.URL,
		},
		log.Instance,
	)

	impressionMockStorage := storageMock.MockImpressionStorage{
		PopNWithMetadataCall: func(n int64) ([]dtos.ImpressionQueueObject, error) {
			if n != 50 {
				t.Error("Wrong input parameter passed")
			}
			return []dtos.ImpressionQueueObject{
				{Impression: wrapImpression("feature1"), Metadata: metadata1},
			}, nil
		},
	}

	impressionSync, _ := NewImpressionRecordMultiple(
		impressionMockStorage,
		impressionRecorder,
		storage.NewMetricWrapper(storageMock.MockMetricStorage{
			IncCounterCall: func(key string) {},
			IncLatencyCall: func(metricName string, index int) {},
		}, nil, nil),
		log.Instance,
		conf.ManagerConfig{
			ImpressionsMode: conf.ImpressionsModeDebug,
			OperationMode:   conf.ProducerSync,
			ListenerEnabled: true,
		},
		nil,
	)

	impressionSync.SynchronizeImpressions(50)
	impressionSync.SynchronizeImpressions(50)

	if requestReceived != 2 {
		t.Error("It should call twice")
	}
}
