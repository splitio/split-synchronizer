package producer

import (
	"context"
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

	routerHealthcheck := gin.Default()
	routerHealthcheck.GET("/test", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	serverHealthcheck := &http.Server{
		Addr:    ":9999",
		Handler: routerHealthcheck,
	}

	go serverHealthcheck.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctxHealthcheck, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/test")
	body, _ := ioutil.ReadAll(res.Body)

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
	serverHealthcheck.Shutdown(ctxHealthcheck)
}

func TestHealthCheckEndpointFailure(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	tsHealthcheck2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Error"))
		fmt.Fprintln(w, "ok")
	}))
	defer tsHealthcheck2.Close()

	os.Setenv("SPLITIO_SDK_URL", tsHealthcheck2.URL)
	os.Setenv("SPLITIO_EVENTS_URL", tsHealthcheck2.URL)

	api.Initialize()

	routerHealthcheck2 := gin.Default()
	routerHealthcheck2.GET("/TestHealthCheckEndpointFailure", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: true})
		HealthCheck(c)
	})

	serverHealthcheck2 := &http.Server{
		Addr:    ":9999",
		Handler: routerHealthcheck2,
	}

	go serverHealthcheck2.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctxHealthcheck2, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/TestHealthCheckEndpointFailure")
	body, _ := ioutil.ReadAll(res.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if gs.Storage.Healthy {
		t.Error("Storage should NOT be healthy")
	}
	serverHealthcheck2.Shutdown(ctxHealthcheck2)
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
	router.GET("/test", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/test")
	body, _ := ioutil.ReadAll(res.Body)

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

	server.Shutdown(ctx)
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
	router.GET("/test", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: false})
		HealthCheck(c)
	})

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/test")
	body, _ := ioutil.ReadAll(res.Body)

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

	server.Shutdown(ctx)
}

func TestSizeEvents(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	redis.Client.Del(eventsStorageAdapter.GetQueueNamespace())

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
		eventsStorageAdapter.GetQueueNamespace(),
		toStore,
	)

	router := gin.Default()
	router.GET("/test", func(c *gin.Context) {
		GetEventsQueueSize(c)
	})

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/test")
	responseBody, _ := ioutil.ReadAll(res.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 1
	if data["queueSize"] != expected {
		t.Error("It should return 1")
	}

	redis.Client.Del(eventsStorageAdapter.GetQueueNamespace())
	server.Shutdown(ctx)
}

func TestSizeImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	redis.Client.Del(impressionsStorageAdapter.GetQueueNamespace())

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
		impressionsStorageAdapter.GetQueueNamespace(),
		toStore,
	)

	router := gin.Default()
	router.GET("/test", func(c *gin.Context) {
		GetImpressionsQueueSize(c)
	})

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9999/test")
	responseBody, _ := ioutil.ReadAll(res.Body)

	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 1
	if data["queueSize"] != expected {
		t.Error("It should return 1")
	}

	redis.Client.Del(impressionsStorageAdapter.GetQueueNamespace())
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test?size=size", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 400 {
		t.Error("Should returned 400")
	}
	if body != "Wrong type passed as parameter" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test?size=-10", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 400 {
		t.Error("Should returned 400")
	}
	if body != "Size cannot be less than 1" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test?size=10", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 200 {
		t.Error("Should returned 200")
	}
	if body != "Events dropped" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 200 {
		t.Error("Should returned 200")
	}
	if body != "Events dropped" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test?size=size", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 400 {
		t.Error("Should returned 400")
	}
	if body != "Wrong type passed as parameter" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test?size=1", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 200 {
		t.Error("Should returned 200")
	}
	if body != "Impressions dropped" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
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

	server := &http.Server{
		Addr:    ":9999",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Post("http://localhost:9999/test", "", nil)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	body := string(bodyBytes)
	if res.StatusCode != 200 {
		t.Error("Should returned 200")
	}
	if body != "Impressions dropped" {
		t.Error("Wrong message")
	}
	server.Shutdown(ctx)
}
