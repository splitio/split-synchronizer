package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
)

type monitorMock struct {
	statusCall func() application.HealthDto
}

func (m *monitorMock) GetHealthStatus() application.HealthDto {
	return m.statusCall()
}

func (m *monitorMock) NotifyEvent(counterType int)      {}
func (m *monitorMock) Reset(counterType int, value int) {}
func (m *monitorMock) Start()                           {}
func (m *monitorMock) Stop()                            {}

func TestApplicationHealthCheckEndpointErr(t *testing.T) {

	appHC := &monitorMock{}
	appHC.statusCall = func() application.HealthDto {
		return application.HealthDto{
			Healthy: false,
		}
	}

	ctrl := NewHealthCheckController(logging.NewLogger(nil), appHC, nil)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/health/application", nil)
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 500 {
		t.Error("status code should be 500.")
	}

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	var result application.HealthDto
	if err := json.Unmarshal(responseBody, &result); err != nil {
		t.Error("there should be no error ", err)
	}
}

func TestApplicationHealthCheckEndpointOk(t *testing.T) {

	appHC := &monitorMock{}
	appHC.statusCall = func() application.HealthDto {
		return application.HealthDto{
			Healthy: true,
		}
	}

	ctrl := NewHealthCheckController(logging.NewLogger(nil), appHC, nil)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/health/application", nil)
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("status code should be 200.")
	}

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	var result application.HealthDto
	if err := json.Unmarshal(responseBody, &result); err != nil {
		t.Error("there should be no error ", err)
	}
}
