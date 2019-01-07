package controllers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

func TestGetConfiguration(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.Redis.ClusterMode = true
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.GET("/test", func(c *gin.Context) {
		GetConfiguration(c)
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

	if data["mode"] != "ProducerMode" {
		t.Error("It should be ProducerMode")
	}

	if data["redisMode"] != "Cluster" {
		t.Error("It should be Cluster")
	}

	server.Shutdown(ctx)
}

func TestGetConfigurationSimple(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	redis.Initialize(conf.Data.Redis)

	router := gin.Default()
	router.GET("/test", func(c *gin.Context) {
		GetConfiguration(c)
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

	if data["mode"] != "ProducerMode" {
		t.Error("It should be ProducerMode")
	}

	if data["redisMode"] != "Simple" {
		t.Error("It should be Simple")
	}

	server.Shutdown(ctx)
}
