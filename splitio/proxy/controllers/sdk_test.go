package controllers

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/service/mocks"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	psmocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"
)

func TestSplitChangesCachedRecipe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{
			FetchCall: func(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
				t.Error("should not be called")
				return nil, nil
			},
		},
		&psmocks.ProxySplitStorageMock{
			ChangesSinceCall: func(since int64) (*dtos.SplitChangesDTO, error) {
				if since != -1 {
					t.Error("since should be -1")
				}

				return &dtos.SplitChangesDTO{
					Since: -1,
					Till:  1,
					Splits: []dtos.SplitDTO{
						{Name: "s1"},
						{Name: "s2"},
					},
				}, nil
			},
			RegisterOlderCnCall: func(payload *dtos.SplitChangesDTO) {
				t.Error("should not be called")
			},
		},
		nil,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var s dtos.SplitChangesDTO
	json.Unmarshal(body, &s)
	if len(s.Splits) != 2 || s.Since != -1 || s.Till != 1 {
		t.Error("wrong payload returned")
	}
}

func TestSplitChangesNonCachedRecipe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{
			FetchCall: func(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
				if changeNumber != -1 {
					t.Error("changeNumber should be -1")
				}

				return &dtos.SplitChangesDTO{
					Since: -1,
					Till:  1,
					Splits: []dtos.SplitDTO{
						{Name: "s1"},
						{Name: "s2"},
					},
				}, nil
			},
		},
		&psmocks.ProxySplitStorageMock{
			ChangesSinceCall: func(since int64) (*dtos.SplitChangesDTO, error) {
				if since != -1 {
					t.Error("since should be -1")
				}
				return nil, storage.ErrSummaryNotCached
			},
			RegisterOlderCnCall: func(payload *dtos.SplitChangesDTO) {
				if payload.Since != -1 || len(payload.Splits) != 2 {
					t.Error("invalid payload passed")
				}
			},
		},
		nil,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var s dtos.SplitChangesDTO
	json.Unmarshal(body, &s)
	if len(s.Splits) != 2 || s.Since != -1 || s.Till != 1 {
		t.Error("wrong payload returned")
	}
}

func TestSplitChangesNonCachedRecipeAndFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{
			FetchCall: func(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
				if changeNumber != -1 {
					t.Error("changeNumber should be -1")
				}
				return nil, errors.New("something")
			},
		},
		&psmocks.ProxySplitStorageMock{
			ChangesSinceCall: func(since int64) (*dtos.SplitChangesDTO, error) {
				if since != -1 {
					t.Error("since should be -1")
				}
				return nil, storage.ErrSummaryNotCached
			},
			RegisterOlderCnCall: func(payload *dtos.SplitChangesDTO) {
				if payload.Since != -1 || len(payload.Splits) != 2 {
					t.Error("invalid payload passed")
				}
			},
		},
		nil,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 500 {
		t.Error("Status code should be 500 and is ", resp.Code)
	}
}

func TestSegmentChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{},
		&psmocks.ProxySplitStorageMock{},
		&psmocks.ProxySegmentStorageMock{
			ChangesSinceCall: func(name string, since int64) (*dtos.SegmentChangesDTO, error) {
				if name != "someSegment" || since != -1 {
					t.Error("wrong params")
				}
				return &dtos.SegmentChangesDTO{
					Name:    "someSegment",
					Added:   []string{"k1", "k2"},
					Removed: []string{},
					Since:   -1,
					Till:    1,
				}, nil
			},
		},
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/segmentChanges/someSegment?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var s dtos.SegmentChangesDTO
	json.Unmarshal(body, &s)
	if s.Name != "someSegment" || len(s.Added) != 2 || len(s.Removed) != 0 || s.Since != -1 || s.Till != 1 {
		t.Error("wrong payload returned")
	}
}

func TestSegmentChangesNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{},
		&psmocks.ProxySplitStorageMock{},
		&psmocks.ProxySegmentStorageMock{
			ChangesSinceCall: func(name string, since int64) (*dtos.SegmentChangesDTO, error) {
				if name != "someSegment" || since != -1 {
					t.Error("wrong params")
				}
				return nil, storage.ErrSegmentNotFound
			},
		},
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/segmentChanges/someSegment?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 404 {
		t.Error("Status code should be 404 and is ", resp.Code)
	}
}

func TestMySegments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{},
		&psmocks.ProxySplitStorageMock{},
		&psmocks.ProxySegmentStorageMock{
			SegmentsForCall: func(key string) ([]string, error) {
				if key != "someKey" {
					t.Error("wrong key")
				}

				return []string{"segment1", "segment2"}, nil
			},
		},
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	type MSC struct {
		MySegments []dtos.MySegmentDTO `json:"mySegments"`
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var ms MSC
	json.Unmarshal(body, &ms)
	s := ms.MySegments
	if len(s) != 2 || s[0].Name != "segment1" || s[1].Name != "segment2" {
		t.Error("invalid payload", s)
	}
}

func TestMySegmentsError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&mocks.MockSplitFetcher{},
		&psmocks.ProxySplitStorageMock{},
		&psmocks.ProxySegmentStorageMock{
			SegmentsForCall: func(key string) ([]string, error) {
				if key != "someKey" {
					t.Error("wrong key")
				}

				return nil, errors.New("something")
			},
		},
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 500 {
		t.Error("Status code should be 500 and is ", resp.Code)
	}
}
