package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks/mocks"
)

func TestPostConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawTelemetryConfig)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if expected != data.Metadata {
					t.Error("wrong metadata")
				}

				var parsed dtos.Config
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error parsing config")
				}

				if !parsed.StreamingEnabled || parsed.OperationMode != 1 || parsed.Storage != "sarasa" {
					t.Error("wrong payload")
				}
				return nil
			},
		},
		&mocks.MockDeferredRecordingTask{},
	)

	group := router.Group("/api")
	controller.Register(group)

	serialized, err := json.Marshal(dtos.Config{
		OperationMode:    1,
		StreamingEnabled: true,
		Storage:          "sarasa",
	})
	if err != nil {
		t.Error("error serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/config", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawTelemetryUsage)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if expected != data.Metadata {
					t.Error("wrong metadata")
				}

				var parsed dtos.Stats
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error parsing config")
				}

				if parsed.AuthRejections != 2 || parsed.TokenRefreshes != 1 {
					t.Error("wrong payload")
				}
				return nil
			},
		},
	)

	group := router.Group("/api")
	controller.Register(group)

	serialized, err := json.Marshal(dtos.Stats{TokenRefreshes: 1, AuthRejections: 2})
	if err != nil {
		t.Error("error serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/usage", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}
