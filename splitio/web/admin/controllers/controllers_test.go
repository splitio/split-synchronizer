package controllers

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v3/dtos"
	apiMocks "github.com/splitio/go-split-commons/v3/service/api/mocks"
	redisStorage "github.com/splitio/go-split-commons/v3/storage/redis"
	"github.com/splitio/go-toolkit/v4/logging"
	"github.com/splitio/go-toolkit/v4/redis"
	"github.com/splitio/go-toolkit/v4/redis/mocks"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
)

const eventsListNamespace = "SPLITIO.events"
const impressionsQueueNamespace = "SPLITIO.impressions"

type itemStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

type date struct {
	Date string `json:"date"`
	Time string `json:"time"`
}

type globalStatus struct {
	Sync         itemStatus  `json:"sync"`
	Storage      *itemStatus `json:"storage"`
	Sdk          itemStatus  `json:"sdk"`
	Events       itemStatus  `json:"events"`
	Auth         itemStatus  `json:"auth"`
	Proxy        *itemStatus `json:"proxy,omitempty"`
	HealthySince date        `json:"healthySince"`
	Uptime       string      `json:"uptime"`
}

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetConfiguration(t *testing.T) {
	conf.Initialize()
	conf.Data.Redis.ClusterMode = true

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(100 * time.Millisecond)
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
	conf.Initialize()

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(100 * time.Millisecond)

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
	appcontext.Initialize(appcontext.ProxyMode)

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		GetConfiguration(c)
	})

	time.Sleep(100 * time.Millisecond)
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
	conf.Initialize()
	redisMock := mocks.MockClient{
		LLenCall: func(key string) redis.Result {
			return &mocks.MockResultOutput{
				ResultCall: func() (int64, error) { return 100, nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.EventStorage, redisStorage.NewEventsStorage(prefixed, dtos.Metadata{}, nil))
		GetEventsQueueSize(c)
	})

	w := performRequest(router, "GET", "/")
	if http.StatusOK != w.Code {
		t.Error("Expected 200", w.Code)
	}

	responseBody, _ := ioutil.ReadAll(w.Body)
	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 100
	if data["queueSize"] != expected {
		t.Error("It should return 100")
	}
}

func TestSizeImpressions(t *testing.T) {
	conf.Initialize()
	redisMock := mocks.MockClient{
		LLenCall: func(key string) redis.Result {
			return &mocks.MockResultOutput{
				ResultCall: func() (int64, error) { return 100, nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.ImpressionStorage, redisStorage.NewImpressionStorage(prefixed, dtos.Metadata{}, nil))
		GetImpressionsQueueSize(c)
	})

	w := performRequest(router, "GET", "/")
	if http.StatusOK != w.Code {
		t.Error("Expected 200")
	}

	responseBody, _ := ioutil.ReadAll(w.Body)
	var data map[string]interface{}
	_ = json.Unmarshal([]byte(responseBody), &data)
	var expected float64 = 100
	if data["queueSize"] != expected {
		t.Error("It should return 100")
	}
}

func TestDropEventsFail(t *testing.T) {
	conf.Initialize()

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

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
	conf.Initialize()

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropEvents(c)
	})

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
	conf.Initialize()
	redisMock := mocks.MockClient{
		LTrimCall: func(key string, start, stop int64) redis.Result {
			if key != eventsListNamespace {
				t.Error("Unexpected key passed")
			}
			if start != 10 && stop != -1 {
				t.Error("Unexpected passed size")
			}
			return &mocks.MockResultOutput{
				ErrCall: func() error { return nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		c.Set(common.EventStorage, redisStorage.NewEventsStorage(prefixed, dtos.Metadata{}, nil))
		DropEvents(c)
	})

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
	conf.Initialize()
	redisMock := mocks.MockClient{
		DelCall: func(keys ...string) redis.Result {
			if keys[0] != eventsListNamespace {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultCall: func() (int64, error) { return 1, nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		c.Set(common.EventStorage, redisStorage.NewEventsStorage(prefixed, dtos.Metadata{}, nil))
		DropEvents(c)
	})

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
	conf.Initialize()

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
	})

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
	conf.Initialize()

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		DropImpressions(c)
	})

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
	conf.Initialize()
	redisMock := mocks.MockClient{
		LTrimCall: func(key string, start, stop int64) redis.Result {
			if key != impressionsQueueNamespace {
				t.Error("Unexpected key passed")
			}
			if start != 1 && stop != -1 {
				t.Error("Unexpected passed size")
			}
			return &mocks.MockResultOutput{
				ErrCall: func() error { return nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		c.Set(common.ImpressionStorage, redisStorage.NewImpressionStorage(prefixed, dtos.Metadata{}, nil))
		DropImpressions(c)
	})

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
	conf.Initialize()
	redisMock := mocks.MockClient{
		DelCall: func(keys ...string) redis.Result {
			if keys[0] != impressionsQueueNamespace {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultCall: func() (int64, error) { return 1, nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		c.Set(common.ImpressionStorage, redisStorage.NewImpressionStorage(prefixed, dtos.Metadata{}, nil))
		DropImpressions(c)
	})

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
	conf.Initialize()

	router := gin.Default()
	router.POST("/test", func(c *gin.Context) {
		FlushImpressions(c)
	})

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

func TestHealthCheckEndpointSuccessful(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
	conf.Initialize()
	redisMock := mocks.MockClient{
		GetCall: func(key string) redis.Result {
			if key != "SPLITIO.splits.till" {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultStringCall: func() (string, error) { return "12", nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.SplitStorage, redisStorage.NewSplitStorage(prefixed, nil))
		c.Set(common.HTTPClientsGin, common.HTTPClients{
			AuthClient:   apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			SdkClient:    apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			EventsClient: apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
		})
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
	if !gs.Auth.Healthy {
		t.Error("Auth should be healthy")
	}
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
	if gs.HealthySince.Date == "0" {
		t.Error("Should be healthy")
	}
}

func TestHealthCheckEndpointFailure(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	redisMock := mocks.MockClient{
		GetCall: func(key string) redis.Result {
			if key != "SPLITIO.splits.till" {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultStringCall: func() (string, error) { return "", errors.New("some") },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.SplitStorage, redisStorage.NewSplitStorage(prefixed, nil))
		c.Set(common.HTTPClientsGin, common.HTTPClients{
			AuthClient:   apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, errors.New("some") }},
			SdkClient:    apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			EventsClient: apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
		})
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
	if gs.HealthySince.Date != "0" {
		t.Error("It should not write since")
	}
}

func TestHealthCheckEndpointSDKFail(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	redisMock := mocks.MockClient{
		GetCall: func(key string) redis.Result {
			if key != "SPLITIO.splits.till" {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultStringCall: func() (string, error) { return "12", nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.SplitStorage, redisStorage.NewSplitStorage(prefixed, nil))
		c.Set(common.HTTPClientsGin, common.HTTPClients{
			AuthClient:   apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			SdkClient:    apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, errors.New("some") }},
			EventsClient: apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
		})
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
	if !gs.Auth.Healthy {
		t.Error("Auth should be healthy")
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
	if gs.HealthySince.Date != "0" {
		t.Error("It should not write since")
	}
}

func TestHealthCheckEndpointEventsFail(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	redisMock := mocks.MockClient{
		GetCall: func(key string) redis.Result {
			if key != "SPLITIO.splits.till" {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultStringCall: func() (string, error) { return "12", nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.SplitStorage, redisStorage.NewSplitStorage(prefixed, nil))
		c.Set(common.HTTPClientsGin, common.HTTPClients{
			AuthClient:   apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			SdkClient:    apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			EventsClient: apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, errors.New("some") }},
		})
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
	if !gs.Auth.Healthy {
		t.Error("Auth should be healthy")
	}
	if !gs.Sdk.Healthy {
		t.Error("Sdk should not be healthy")
	}
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
	if gs.HealthySince.Date != "0" {
		t.Error("Should be 0")
	}
}

func TestHealthCheckEndpointAuthFail(t *testing.T) {
	appcontext.Initialize(appcontext.ProducerMode)
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	redisMock := mocks.MockClient{
		GetCall: func(key string) redis.Result {
			if key != "SPLITIO.splits.till" {
				t.Error("Unexpected key")
			}
			return &mocks.MockResultOutput{
				ResultStringCall: func() (string, error) { return "12", nil },
			}
		},
	}
	prefixed, _ := redis.NewPrefixedRedisClient(&redisMock, "")

	router := gin.Default()
	router.GET("/", func(c *gin.Context) {
		c.Set(common.SplitStorage, redisStorage.NewSplitStorage(prefixed, nil))
		c.Set(common.HTTPClientsGin, common.HTTPClients{
			AuthClient:   apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, errors.New("some") }},
			SdkClient:    apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
			EventsClient: apiMocks.ClientMock{GetCall: func(string, map[string]string) ([]byte, error) { return []byte{}, nil }},
		})
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
	if gs.Auth.Healthy {
		t.Error("Auth should be healthy")
	}
	if !gs.Sdk.Healthy {
		t.Error("Sdk should not be healthy")
	}
	if gs.Proxy != nil {
		t.Error("Should not be status for proxy mode")
	}
	if gs.HealthySince.Date != "0" {
		t.Error("Should be 0")
	}
}
