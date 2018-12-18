package producer

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
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
	server.Shutdown(ctx)
}

func TestHealthCheckEndpointFailure(t *testing.T) {
	router := gin.Default()
	router.GET("/test", func(c *gin.Context) {
		c.Set("SplitStorage", mockStorage{shouldFail: true})
		HealthCheck(c)
	})

	server := &http.Server{
		Addr:    ":9998",
		Handler: router,
	}

	go server.ListenAndServe()
	time.Sleep(3 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, _ := http.Get("http://localhost:9998/test")
	body, _ := ioutil.ReadAll(res.Body)

	gs := globalStatus{}
	json.Unmarshal(body, &gs)
	if gs.Storage.Healthy {
		t.Error("Storage should NOT be healthy")
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

	res, _ := http.Post("http://localhost:9999/test?size=1", "", nil)
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
