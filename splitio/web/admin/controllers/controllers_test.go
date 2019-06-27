package controllers

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
	"github.com/splitio/split-synchronizer/appcontext"
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
	Sync         itemStatus  `json:"sync"`
	Storage      *itemStatus `json:"storage"`
	Sdk          itemStatus  `json:"sdk"`
	Events       itemStatus  `json:"events"`
	Proxy        *itemStatus `json:"proxy,omitempty"`
	HealthySince string      `json:"healthySince"`
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

func TestGetConfiguration(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.Redis.ClusterMode = true
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(1 * time.Second)
	w := performRequest(router, "GET", "/")

	if http.StatusOK != w.Code {
		t.Error("Expected 200")
	}

	responseBody, _ := ioutil.ReadAll(w.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)

	if data["mode"] != "ProducerMode" {
		t.Error("It should be ProducerMode")
	}

	if data["redisMode"] != "Cluster" {
		t.Error("It should be Cluster")
	}

	if data["redis"] == nil {
		t.Error("Should have config")
	}

	if data["proxy"] != nil {
		t.Error("Should not have config")
	}
}

func TestGetConfigurationSimple(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(1 * time.Second)

	w := performRequest(router, "GET", "/")

	responseBody, _ := ioutil.ReadAll(w.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)

	if data["mode"] != "ProducerMode" {
		t.Error("It should be ProducerMode")
	}

	if data["redisMode"] != "Standard" {
		t.Error("It should be Standard")
	}

	if data["redis"] == nil {
		t.Error("Should have config")
	}

	if data["proxy"] != nil {
		t.Error("Should not have config")
	}
}

func TestGetConfigurationProxyMode(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	appcontext.Initialize(appcontext.ProxyMode)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(1 * time.Second)
	w := performRequest(router, "GET", "/")

	responseBody, _ := ioutil.ReadAll(w.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)

	if data["mode"] != "ProxyMode" {
		t.Error("It should be ProxyMode")
	}

	if data["redis"] != nil {
		t.Error("Should not have config")
	}

	if data["proxy"] == nil {
		t.Error("Should have config")
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

	redis.Client.RPush(
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

	redis.Client.RPush(
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

func TestDropImpressionsFailSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
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

func TestAnotherOperationRunningOnEvents(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []api.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		time.Sleep(3 * time.Second)

		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()
	conf.Initialize()
	conf.Data.Redis.Prefix = "testflush"

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
	eventBulk := make([]interface{}, itemsToAdd)
	for i := 0; i < itemsToAdd; i++ {
		eventBulk[i] = eventJSON
	}
	redis.Client.RPush(eventListName, eventBulk...)

	//----------------

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()

		router := gin.Default()
		router.POST("/flushEvents", func(c *gin.Context) {
			FlushEvents(c)
		})

		router.POST("/dropEvents", func(c *gin.Context) {
			DropEvents(c)
		})

		time.Sleep(3 * time.Second)

		res1 := make(chan int)
		res2 := make(chan int)
		res3 := make(chan int)
		res2Msg := make(chan string)
		res3Msg := make(chan string)

		go func() {
			res := performRequest(router, "POST", "/flushEvents")
			res1 <- res.Code
		}()
		go func() {
			time.Sleep(300 * time.Millisecond)
			res := performRequest(router, "POST", "/dropEvents")
			res2 <- res.Code
			bodyBytes, _ := ioutil.ReadAll(res.Body)
			res2Msg <- string(bodyBytes)
		}()
		go func() {
			time.Sleep(400 * time.Millisecond)
			res := performRequest(router, "POST", "/flushEvents")
			res3 <- res.Code
			bodyBytes, _ := ioutil.ReadAll(res.Body)
			res3Msg <- string(bodyBytes)
		}()

		x := <-res1
		y := <-res2
		yMsg := <-res2Msg
		z := <-res3
		zMsg := <-res3Msg
		if x != http.StatusOK {
			t.Error("Should returned 200")
		}

		if y != http.StatusInternalServerError {
			t.Error("Should returned 500")
		}
		if yMsg != "Cannot execute drop. Another operation is performing operations on Events" {
			t.Error("Wrong message")
		}

		if z != http.StatusInternalServerError {
			t.Error("Should returned 500")
		}
		if zMsg != "Cannot execute flush. Another operation is performing operations on Events" {
			t.Error("Wrong message")
		}

		res := performRequest(router, "POST", "/dropEvents")
		if res.Code != http.StatusOK {
			t.Error("Should returned 200")
		}

		res = performRequest(router, "POST", "/dropEvents")
		if res.Code != http.StatusOK {
			t.Error("Should returned 200")
		}
	}()
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

func TestAnotherOperationRunningOnImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsInPost []redis.ImpressionDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		time.Sleep(3 * time.Second)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()
	conf.Initialize()
	conf.Data.Redis.Prefix = "impressionstest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 10003
	impressionListName := conf.Data.Redis.Prefix + ".SPLITIO.impressions"

	impressionJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"i":{"k":"6c4829ab-a0d8-4e72-8176-a334f596fb79","b":"bucketing","f":"feature","t":"ON","c":12345,"r":"rule","timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(impressionListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 10003 impressions
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(impressionListName, impressionJSON)
	}

	//----------------

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()

		router := gin.Default()
		router.POST("/flushImpressions", func(c *gin.Context) {
			FlushImpressions(c)
		})

		router.POST("/dropImpressions", func(c *gin.Context) {
			DropImpressions(c)
		})

		time.Sleep(3 * time.Second)

		res1 := make(chan int)
		res2 := make(chan int)
		res3 := make(chan int)
		res2Msg := make(chan string)
		res3Msg := make(chan string)

		go func() {
			res := performRequest(router, "POST", "/flushImpressions")
			res1 <- res.Code
		}()
		go func() {
			time.Sleep(300 * time.Millisecond)
			res := performRequest(router, "POST", "/dropImpressions")
			bodyBytes, _ := ioutil.ReadAll(res.Body)
			res2 <- res.Code
			res2Msg <- string(bodyBytes)
		}()
		go func() {
			time.Sleep(400 * time.Millisecond)
			res := performRequest(router, "POST", "/flushImpressions")
			bodyBytes, _ := ioutil.ReadAll(res.Body)
			res3 <- res.Code
			res3Msg <- string(bodyBytes)
		}()

		x := <-res1
		y := <-res2
		yMsg := <-res2Msg
		z := <-res3
		zMsg := <-res3Msg
		if x != http.StatusOK {
			t.Error("Should returned 200")
		}

		if y != http.StatusInternalServerError {
			t.Error("Should returned 500")
		}
		if yMsg != "Cannot execute drop. Another operation is performing operations on Impressions" {
			t.Error("Wrong message")
		}

		if z != http.StatusInternalServerError {
			t.Error("Should returned 500")
		}
		if zMsg != "Cannot execute flush. Another operation is performing operations on Impressions" {
			t.Error("Wrong message")
			t.Error(zMsg)
		}

		res := performRequest(router, "POST", "/dropImpressions")
		if res.Code != http.StatusOK {
			t.Error("Should returned 200")
		}

		res = performRequest(router, "POST", "/dropImpressions")
		if res.Code != http.StatusOK {
			t.Error("Should returned 200")
		}
	}()
}

func TestHealthCheckEndpointSuccessful(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
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
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
}

func TestHealthCheckEndpointFailure(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
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
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
	if gs.HealthySince != "" {
		t.Error("It should not write since")
	}
}

func TestHealthCheckEndpointSDKFail(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
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
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
	if gs.HealthySince != "" {
		t.Error("It should not write since")
	}
}

func TestHealthCheckEndpointEventsFail(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
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
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
}

func TestHealtcheckEndpointProxy(t *testing.T) {
	appcontext.Initialize(appcontext.ProxyMode)
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

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

	if !gs.Events.Healthy {
		t.Error("Events should be healthy")
	}
	if !gs.Sdk.Healthy {
		t.Error("Sdk should be healthy")
	}
	if gs.Proxy == nil {
		t.Error("Should return status for proxy mode")
	}
	if gs.Storage != nil {
		t.Error("Should not be status for producer mode")
	}
}
