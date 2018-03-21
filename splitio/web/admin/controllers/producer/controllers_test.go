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
