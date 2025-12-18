package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks/mocks"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
)

func TestPostConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

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
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		apikeyValidator.IsValid,
	)

	group := router.Group("/api")
	controller.Register(group, group)

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
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

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
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		apikeyValidator.IsValid,
	)

	group := router.Group("/api")
	controller.Register(group, group)

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

type UniquesCS struct {
	Keys []KeyCS `json:"keys,omitempty"`
}

type KeyCS struct {
	Features []string `json:"fs,omitempty"`
	Key      string   `json:"k,omitempty"`
}

func TestPostKeysClientSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawKeysClientSide)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if expected != data.Metadata {
					t.Error("wrong metadata")
				}

				var parsed UniquesCS
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error parsing config")
				}

				if len(parsed.Keys) != 2 {
					t.Error("It should parse two keys")
				}

				keysMap := make(map[string][]string)
				for _, key := range parsed.Keys {
					keysMap[key.Key] = key.Features
				}

				if keysMap["key-1"] == nil {
					t.Error("key-1 should exists")
				}
				if keysMap["key-1"][0] != "feature-1" || keysMap["key-1"][1] != "feature-2" {
					t.Error("Wrong payload")
				}

				if keysMap["key-2"] == nil {
					t.Error("key-2 should exists")
				}
				if keysMap["key-2"][0] != "feature-1" || keysMap["key-2"][1] != "feature-3" {
					t.Error("Wrong payload")
				}

				return nil
			},
		},
		&mocks.MockDeferredRecordingTask{},
		apikeyValidator.IsValid,
	)

	group := router.Group("/api")
	controller.Register(group, group)

	serialized, err := json.Marshal(UniquesCS{
		Keys: []KeyCS{
			{Features: []string{"feature-1", "feature-2"}, Key: "key-1"},
			{Features: []string{"feature-1", "feature-3"}, Key: "key-2"},
		},
	})
	if err != nil {
		t.Error("error serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/keys/cs", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostKeysServerSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawKeysServerSide)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if expected != data.Metadata {
					t.Error("wrong metadata")
				}

				var parsed dtos.Uniques
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error parsing config")
				}

				if len(parsed.Keys) != 2 {
					t.Error("It should parse two keys")
				}

				keysMap := make(map[string][]string)
				for _, key := range parsed.Keys {
					keysMap[key.Feature] = key.Keys
				}

				if keysMap["feature-1"] == nil {
					t.Error("feature-1 should exists")
				}
				if keysMap["feature-1"][0] != "key-1" || keysMap["feature-1"][1] != "key-2" {
					t.Error("Wrong payload")
				}

				if keysMap["feature-2"] == nil {
					t.Error("feature-2 should exists")
				}
				if keysMap["feature-2"][0] != "key-1" || keysMap["feature-2"][1] != "key-3" {
					t.Error("Wrong payload")
				}

				return nil
			},
		},
		apikeyValidator.IsValid,
	)

	group := router.Group("/api")
	controller.Register(group, group)

	serialized, err := json.Marshal(dtos.Uniques{
		Keys: []dtos.Key{
			{Feature: "feature-1", Keys: []string{"key-1", "key-2"}},
			{Feature: "feature-2", Keys: []string{"key-1", "key-3"}},
		},
	})
	if err != nil {
		t.Error("error serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/keys/ss", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostBeaconKeysClientSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawKeysClientSide)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "NA", MachineName: "NA"}
				if expected != data.Metadata {
					t.Error("wrong metadata")
				}

				var parsed UniquesCS
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error parsing config")
				}

				if len(parsed.Keys) != 2 {
					t.Error("It should parse two keys")
				}

				keysMap := make(map[string][]string)
				for _, key := range parsed.Keys {
					keysMap[key.Key] = key.Features
				}

				if keysMap["key-1"] == nil {
					t.Error("key-1 should exists")
				}
				if keysMap["key-1"][0] != "feature-1" || keysMap["key-1"][1] != "feature-2" {
					t.Error("Wrong payload")
				}

				if keysMap["key-2"] == nil {
					t.Error("key-2 should exists")
				}
				if keysMap["key-2"][0] != "feature-1" || keysMap["key-2"][1] != "feature-3" {
					t.Error("Wrong payload")
				}

				return nil
			},
		},
		&mocks.MockDeferredRecordingTask{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	entries, err := json.Marshal(UniquesCS{
		Keys: []KeyCS{
			{Features: []string{"feature-1", "feature-2"}, Key: "key-1"},
			{Features: []string{"feature-1", "feature-3"}, Key: "key-2"},
		},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	serialized, err := json.Marshal(beaconMessage{Entries: entries, Sdk: "go-1.1.1", Token: "someApiKey"})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/keys/cs/beacon", bytes.NewBuffer(serialized))
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 204 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostUsageBeacon(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := middleware.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewTelemetryServerController(
		logger,
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{
			StageCall: func(raw interface{}) error {
				data, ok := raw.(*internal.RawTelemetryUsage)
				if !ok {
					t.Error("failed type assertion")
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "NA", MachineName: "NA"}
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
		&mocks.MockDeferredRecordingTask{},
		&mocks.MockDeferredRecordingTask{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	entries, err := json.Marshal(dtos.Stats{TokenRefreshes: 1, AuthRejections: 2})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	serialized, err := json.Marshal(beaconMessage{Entries: entries, Sdk: "go-1.1.1", Token: "someApiKey"})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/usage/beacon", bytes.NewBuffer(serialized))
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 204 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}
