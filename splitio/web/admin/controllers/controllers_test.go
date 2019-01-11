package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

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
