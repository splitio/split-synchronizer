package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	ilMock "github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener/mocks"
	mw "github.com/splitio/split-synchronizer/v5/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks/mocks"
)

func TestPostImpressionsbulk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawImpressions)

				if data.Mode != "optimized" {
					t.Error("mode should be optimized. Is: ", data.Mode)
				}

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed []dtos.ImpressionsDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data")
					return nil
				}
				if len(parsed) != 2 {
					t.Error("incorect number of events received")
				}

				t1 := parsed[0]
				if t1.TestName != "test1" || len(t1.KeyImpressions) != 3 {
					t.Error("wrong test or impressions amount")
				}

				t2 := parsed[1]
				if t2.TestName != "test2" || len(t2.KeyImpressions) != 4 {
					t.Error("wrong test or impressions amount")
				}

				return nil
			},
		}, // impssions
		&mocks.MockDeferredRecordingTask{}, // imp counts
		&mocks.MockDeferredRecordingTask{}, // events
		&ilMock.ImpressionBulkListenerMock{
			SubmitCall: func(imps []impressionlistener.ImpressionsForListener, metadata *dtos.Metadata) error {
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if *metadata != expected {
					t.Error("wrong metadata")
				}

				if len(imps) != 2 || len(imps[0].KeyImpressions) != 3 || len(imps[1].KeyImpressions) != 4 {
					t.Error("wrong payload passed to impressions listener")
				}
				return nil
			},
		},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	serialized, err := json.Marshal([]dtos.ImpressionsDTO{
		{
			TestName: "test1",
			KeyImpressions: []dtos.ImpressionDTO{
				{KeyName: "k1", Treatment: "on", Time: 1, ChangeNumber: 2, Label: "l1", BucketingKey: "b1", Pt: 0},
				{KeyName: "k2", Treatment: "on", Time: 2, ChangeNumber: 3, Label: "l2", BucketingKey: "b2", Pt: 0},
				{KeyName: "k3", Treatment: "on", Time: 3, ChangeNumber: 4, Label: "l3", BucketingKey: "b3", Pt: 0},
			},
		},
		{
			TestName: "test2",
			KeyImpressions: []dtos.ImpressionDTO{
				{KeyName: "k1", Treatment: "off", Time: 1, ChangeNumber: 2, Label: "l1", BucketingKey: "b1", Pt: 0},
				{KeyName: "k2", Treatment: "off", Time: 2, ChangeNumber: 3, Label: "l2", BucketingKey: "b2", Pt: 0},
				{KeyName: "k3", Treatment: "off", Time: 3, ChangeNumber: 4, Label: "l3", BucketingKey: "b3", Pt: 0},
				{KeyName: "k4", Treatment: "off", Time: 4, ChangeNumber: 5, Label: "l4", BucketingKey: "b4", Pt: 0},
			},
		},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/testImpressions/bulk", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKImpressionsMode", "optimized")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostEventsBulk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{}, // impssions
		&mocks.MockDeferredRecordingTask{}, // imp counts
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawEvents)
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed []dtos.EventDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data")
					return nil
				}
				if len(parsed) != 3 {
					t.Error("incorect number of events received")
				}
				return nil
			},
		}, // events
		&ilMock.ImpressionBulkListenerMock{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	serialized, err := json.Marshal([]dtos.EventDTO{
		{Key: "k1", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
		{Key: "k2", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
		{Key: "k3", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/events/bulk", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostImpressionsCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{}, // impssions
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawImpressionCount)
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed dtos.ImpressionsCountDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data")
					return nil
				}
				if len(parsed.PerFeature) != 3 {
					t.Error("incorect number of events received")
				}
				return nil
			},
		}, // imp counts
		&mocks.MockDeferredRecordingTask{}, // events
		&ilMock.ImpressionBulkListenerMock{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	serialized, err := json.Marshal(dtos.ImpressionsCountDTO{
		PerFeature: []dtos.ImpressionsInTimeFrameDTO{
			{FeatureName: "f1", TimeFrame: 1, RawCount: 1},
			{FeatureName: "f2", TimeFrame: 2, RawCount: 2},
			{FeatureName: "f3", TimeFrame: 3, RawCount: 3},
		},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/testImpressions/count", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostLegacyMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{}, // impssions
		&mocks.MockDeferredRecordingTask{}, // imp counts
		&mocks.MockDeferredRecordingTask{}, // events
		&ilMock.ImpressionBulkListenerMock{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/counter", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/counters", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/time", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/times", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/metrics/gauge", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostBeaconImpressionsbulk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawImpressions)

				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "NA", MachineName: "NA"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed []dtos.ImpressionsDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data", err, "--", string(data.Payload))
					return nil
				}
				if len(parsed) != 2 {
					t.Error("incorect number of events received")
				}

				t1 := parsed[0]
				if t1.TestName != "test1" || len(t1.KeyImpressions) != 3 {
					t.Error("wrong test or impressions amount")
				}

				t2 := parsed[1]
				if t2.TestName != "test2" || len(t2.KeyImpressions) != 4 {
					t.Error("wrong test or impressions amount")
				}

				return nil
			},
		}, // impssions
		&mocks.MockDeferredRecordingTask{}, // imp counts
		&mocks.MockDeferredRecordingTask{}, // events
		&ilMock.ImpressionBulkListenerMock{
			SubmitCall: func(imps []impressionlistener.ImpressionsForListener, metadata *dtos.Metadata) error {
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "ip-1-2-3-4"}
				if *metadata != expected {
					t.Error("wrong metadata")
				}

				if len(imps) != 2 || len(imps[0].KeyImpressions) != 3 || len(imps[1].KeyImpressions) != 4 {
					t.Error("wrong payload passed to impressions listener")
				}
				return nil
			},
		},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	entries, err := json.Marshal([]dtos.ImpressionsDTO{
		{
			TestName: "test1",
			KeyImpressions: []dtos.ImpressionDTO{
				{KeyName: "k1", Treatment: "on", Time: 1, ChangeNumber: 2, Label: "l1", BucketingKey: "b1", Pt: 0},
				{KeyName: "k2", Treatment: "on", Time: 2, ChangeNumber: 3, Label: "l2", BucketingKey: "b2", Pt: 0},
				{KeyName: "k3", Treatment: "on", Time: 3, ChangeNumber: 4, Label: "l3", BucketingKey: "b3", Pt: 0},
			},
		},
		{
			TestName: "test2",
			KeyImpressions: []dtos.ImpressionDTO{
				{KeyName: "k1", Treatment: "off", Time: 1, ChangeNumber: 2, Label: "l1", BucketingKey: "b1", Pt: 0},
				{KeyName: "k2", Treatment: "off", Time: 2, ChangeNumber: 3, Label: "l2", BucketingKey: "b2", Pt: 0},
				{KeyName: "k3", Treatment: "off", Time: 3, ChangeNumber: 4, Label: "l3", BucketingKey: "b3", Pt: 0},
				{KeyName: "k4", Treatment: "off", Time: 4, ChangeNumber: 5, Label: "l4", BucketingKey: "b4", Pt: 0},
			},
		},
	})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	serialized, err := json.Marshal(beaconMessage{Entries: entries, Sdk: "go-1.1.1", Token: "someApiKey"})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/testImpressions/beacon", bytes.NewBuffer(serialized))
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKImpressionsMode", "optimized")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 204 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostBeaconEventsBulk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{}, // impssions
		&mocks.MockDeferredRecordingTask{}, // imp counts
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawEvents)
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "NA", MachineName: "NA"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed []dtos.EventDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data")
					return nil
				}
				if len(parsed) != 3 {
					t.Error("incorect number of events received")
				}
				return nil
			},
		}, // events
		&ilMock.ImpressionBulkListenerMock{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	entries, err := json.Marshal([]dtos.EventDTO{
		{Key: "k1", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
		{Key: "k2", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
		{Key: "k3", TrafficTypeName: "tt1", EventTypeID: "e1", Value: 1, Timestamp: 123},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	serialized, err := json.Marshal(beaconMessage{Entries: entries, Sdk: "go-1.1.1", Token: "someApiKey"})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/events/beacon", bytes.NewBuffer(serialized))
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 204 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}

func TestPostBeaconImpressionsCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)
	apikeyValidator := mw.NewAPIKeyValidator([]string{"someApiKey"})

	group := router.Group("/api")
	controller := NewEventsServerController(
		logger,
		&mocks.MockDeferredRecordingTask{}, // impssions
		&mocks.MockDeferredRecordingTask{
			StageCall: func(rawData interface{}) error {
				data := rawData.(*internal.RawImpressionCount)
				expected := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "NA", MachineName: "NA"}
				if data.Metadata != expected {
					t.Error("wrong metadata", expected, data.Metadata)
				}

				var parsed dtos.ImpressionsCountDTO
				err := json.Unmarshal(data.Payload, &parsed)
				if err != nil {
					t.Error("error deserializing incoming data")
					return nil
				}
				if len(parsed.PerFeature) != 3 {
					t.Error("incorect number of events received")
				}
				return nil
			},
		}, // imp counts
		&mocks.MockDeferredRecordingTask{}, // events
		&ilMock.ImpressionBulkListenerMock{},
		apikeyValidator.IsValid,
	)
	controller.Register(group, group)

	entries, err := json.Marshal(dtos.ImpressionsCountDTO{
		PerFeature: []dtos.ImpressionsInTimeFrameDTO{
			{FeatureName: "f1", TimeFrame: 1, RawCount: 1},
			{FeatureName: "f2", TimeFrame: 2, RawCount: 2},
			{FeatureName: "f3", TimeFrame: 3, RawCount: 3},
		},
	})

	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	serialized, err := json.Marshal(beaconMessage{Entries: entries, Sdk: "go-1.1.1", Token: "someApiKey"})
	if err != nil {
		t.Error("should not have errors when serializing: ", err)
	}

	ctx.Request, _ = http.NewRequest(http.MethodPost, "/api/testImpressions/count/beacon", bytes.NewBuffer(serialized))
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 204 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}
}
