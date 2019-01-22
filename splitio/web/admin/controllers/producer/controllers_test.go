package producer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

//Events
const eventsListNamespace = "SPLITIO.events"

//Impressions
const impressionsQueueNamespace = "SPLITIO.impressions"

type itemStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

type globalStatus struct {
	Sync    itemStatus `json:"sync"`
	Storage itemStatus `json:"storage"`
	Sdk     itemStatus `json:"sdk"`
	Events  itemStatus `json:"events"`
}

type mockStorage struct {
	shouldFail bool
}

func (m mockStorage) ChangeNumber() (int64, error) {
	if m.shouldFail {
		return 0, errors.New("X")
	}
	return 1234, nil
}

func (m mockStorage) Save(split interface{}) error             { return nil }
func (m mockStorage) Remove(split interface{}) error           { return nil }
func (m mockStorage) RegisterSegment(name string) error        { return nil }
func (m mockStorage) SetChangeNumber(changeNumber int64) error { return nil }
func (m mockStorage) SplitsNames() ([]string, error)           { return nil, nil }
func (m mockStorage) RawSplits() ([]string, error)             { return nil, nil }

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHealthCheckEndpointSuccessful(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	tsHealthcheck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer tsHealthcheck.Close()

	os.Setenv("SPLITIO_SDK_URL", tsHealthcheck.URL)
	os.Setenv("SPLITIO_EVENTS_URL", tsHealthcheck.URL)

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	w := performRequest(router, "GET", "/")

	if http.StatusOK != w.Code {
		t.Error("Expected 200")
	}

	body, _ := ioutil.ReadAll(w.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if !gs.Storage.Healthy {
		t.Error("Storage should be healthy")
	}
	if !gs.Events.Healthy {
		t.Error("Events should be healthy")
	}
	if !gs.Sdk.Healthy {
		t.Error("Sdk should be healthy")
	}
}

func TestHealthCheckEndpointFailure(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: true})
		HealthCheck(c)
	})

	w := performRequest(router, "GET", "/")

	if http.StatusInternalServerError != w.Code {
		t.Error("Expected 500")
	}

	body, _ := ioutil.ReadAll(w.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if gs.Storage.Healthy {
		t.Error("Storage should NOT be healthy")
	}
}

func TestHealthCheckEndpointSDKFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer fail.Close()

	os.Setenv("SPLITIO_SDK_URL", fail.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	w := performRequest(router, "GET", "/")

	if http.StatusInternalServerError != w.Code {
		t.Error("Expected 500")
	}

	body, _ := ioutil.ReadAll(w.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if !gs.Storage.Healthy {
		t.Error("Storage should be healthy")
	}
	if !gs.Events.Healthy {
		t.Error("Events should be healthy")
	}
	if gs.Sdk.Healthy {
		t.Error("Sdk should not be healthy")
	}
}

func TestHealthCheckEndpointEventsFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer fail.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", fail.URL)

	api.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	w := performRequest(router, "GET", "/")

	if http.StatusInternalServerError != w.Code {
		t.Error("Expected 500")
	}

	body, _ := ioutil.ReadAll(w.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if !gs.Storage.Healthy {
		t.Error("Storage should be healthy")
	}
	if gs.Events.Healthy {
		t.Error("Events should not be healthy")
	}
	if !gs.Sdk.Healthy {
		t.Error("Sdk should not be healthy")
	}
}

func TestSizeEvents(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)
	redis.Client.Del(eventsListNamespace)

	metadata := api.SdkMetadata{
		SdkVersion:  "test-2.0",
		MachineIP:   "127.0.0.1",
		MachineName: "ip-127-0-0-1",
	}

	toStore, err := json.Marshal(api.RedisStoredEventDTO{
		Event: api.EventDTO{
			Key:             "test",
			EventTypeID:     "test",
			Timestamp:       1234,
			TrafficTypeName: "test",
			Value:           nil,
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

	redis.Client.LPush(
		eventsListNamespace,
		toStore,
	)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetEventsQueueSize(c)
	})

	time.Sleep(3 * time.Second)
	w := performRequest(router, "GET", "/")

	if http.StatusOK != w.Code {
		t.Error("Expected 200")
	}

	responseBody, _ := ioutil.ReadAll(w.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 1
	if data["queueSize"] != expected {
		t.Error("It should return 1")
	}

	redis.Client.Del(eventsListNamespace)
}

func TestSizeImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)
	redis.Client.Del(impressionsQueueNamespace)

	metadata := api.SdkMetadata{
		SdkVersion:  "test-2.0",
		MachineIP:   "127.0.0.1",
		MachineName: "ip-127-0-0-1",
	}

	toStore, err := json.Marshal(redis.ImpressionDTO{
		Data: redis.ImpressionObject{
			BucketingKey:      "1",
			FeatureName:       "1",
			KeyName:           "test",
			Rule:              "test",
			SplitChangeNumber: 1234,
			Timestamp:         1234,
			Treatment:         "on",
		},
		Metadata: redis.ImpressionMetadata{
			InstanceIP:   metadata.MachineIP,
			InstanceName: metadata.MachineName,
			SdkVersion:   metadata.SdkVersion,
		},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	redis.Client.LPush(
		impressionsQueueNamespace,
		toStore,
	)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetImpressionsQueueSize(c)
	})

	time.Sleep(3 * time.Second)
	w := performRequest(router, "GET", "/")

	if http.StatusOK != w.Code {
		t.Error("Expected 200")
	}

	responseBody, _ := ioutil.ReadAll(w.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 1
	if data["queueSize"] != expected {
		t.Error("It should return 1")
	}

	redis.Client.Del(impressionsQueueNamespace)
}

func TestDropEventsFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=size")

	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}
	if body != "Wrong type passed as parameter" {
		t.Error("Wrong message")
	}
}

func TestDropEventsFailSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=-10")

	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}
	if body != "Size cannot be less than 1" {
		t.Error("Wrong message")
	}
}

func TestDropEventsSuccess(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=10")

	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusOK {
		t.Error("Should returned 200")
	}
	if body != "Events dropped" {
		t.Error("Wrong message")
	}
}

func TestDropEventsSuccessDefault(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusOK {
		t.Error("Should returned 200")
	}
	if body != "Events dropped" {
		t.Error("Wrong message")
	}
}

func TestDropImpressionsFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=size")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}
	if body != "Wrong type passed as parameter" {
		t.Error("Wrong message")
	}
}

func TestDropImpressionsSuccess(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=1")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusOK {
		t.Error("Should returned 200")
	}
	if body != "Impressions dropped" {
		t.Error("Wrong message")
	}
}

func TestDropImpressionsSuccessDefault(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusOK {
		t.Error("Should returned 200")
	}
	if body != "Impressions dropped" {
		t.Error("Wrong message")
	}
}

func TestFlushImpressionsFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		FlushImpressions(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=200000")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}
	if body != "Max Size to Flush is 25000" {
		t.Error("Wrong message")
	}
}

func TestFlushEventsFail(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		FlushEvents(c)
	})

	time.Sleep(3 * time.Second)
	res := performRequest(router, "POST", "/test?size=200000")
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}
	if body != "Max Size to Flush is 25000" {
		t.Error("Wrong message")
	}
}
