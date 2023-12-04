package controllers

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/service"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	psmocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"
)

func TestSplitChangesRecentSince(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	var splitFetcher splitFetcherMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	logger := logging.NewLogger(nil)
	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&splitFetcher,
		&splitStorage,
		nil,
		flagsets.NewMatcher(false, nil),
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesOlderSince(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return((*dtos.SplitChangesDTO)(nil), storage.ErrSinceParamTooOld).
		Once()

	var splitFetcher splitFetcherMock
	splitFetcher.On("Fetch", int64(-1), ref(service.NewFetchOptions(true, nil))).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&splitFetcher,
		&splitStorage,
		nil,
		flagsets.NewMatcher(false, nil),
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesOlderSinceFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return((*dtos.SplitChangesDTO)(nil), storage.ErrSinceParamTooOld).
		Once()

	var splitFetcher splitFetcherMock
	splitFetcher.On("Fetch", int64(-1), ref(service.NewFetchOptions(true, nil))).
		Return((*dtos.SplitChangesDTO)(nil), errors.New("something")).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&splitFetcher,
		&splitStorage,
		nil,
		flagsets.NewMatcher(false, nil),
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 500, resp.Code)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesWithFlagSets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string{"a", "b", "c"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	var splitFetcher splitFetcherMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&splitFetcher,
		&splitStorage,
		nil,
		flagsets.NewMatcher(false, nil),
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1&sets=c,b,b,a", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	assert.Nil(t, json.Unmarshal(body, &s))
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesWithFlagSetsStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string{"a", "c"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	var splitFetcher splitFetcherMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		&splitFetcher,
		&splitStorage,
		nil,
		flagsets.NewMatcher(true, []string{"a", "c"}),
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1&sets=c,b,b,a", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	assert.Nil(t, json.Unmarshal(body, &s))
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSegmentChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitFetcher splitFetcherMock
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("ChangesSince", "someSegment", int64(-1)).
		Return(&dtos.SegmentChangesDTO{Name: "someSegment", Added: []string{"k1", "k2"}, Removed: []string{}, Since: -1, Till: 1}, nil).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, &splitFetcher, &splitStorage, &segmentStorage, flagsets.NewMatcher(false, nil))
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/segmentChanges/someSegment?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SegmentChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)

	assert.Equal(t, dtos.SegmentChangesDTO{Name: "someSegment", Added: []string{"k1", "k2"}, Removed: []string{}, Since: -1, Till: 1}, s)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

func TestSegmentChangesNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitFetcher splitFetcherMock
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("ChangesSince", "someSegment", int64(-1)).
		Return((*dtos.SegmentChangesDTO)(nil), storage.ErrSegmentNotFound).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, &splitFetcher, &splitStorage, &segmentStorage, flagsets.NewMatcher(false, nil))
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/segmentChanges/someSegment?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 404, resp.Code)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

func TestMySegments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitFetcher splitFetcherMock
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "someKey").
		Return([]string{"segment1", "segment2"}, nil).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, &splitFetcher, &splitStorage, &segmentStorage, flagsets.NewMatcher(false, nil))
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 200, resp.Code)

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	var ms MSC
	err = json.Unmarshal(body, &ms)
	assert.Nil(t, err)

	assert.Equal(t, MSC{MySegments: []dtos.MySegmentDTO{{Name: "segment1"}, {Name: "segment2"}}}, ms)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

func TestMySegmentsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitFetcher splitFetcherMock
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "someKey").
		Return([]string(nil), errors.New("something")).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, &splitFetcher, &splitStorage, &segmentStorage, flagsets.NewMatcher(false, nil))
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 500, resp.Code)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

type splitFetcherMock struct {
	mock.Mock
}

// Fetch implements service.SplitFetcher
func (s *splitFetcherMock) Fetch(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
	args := s.Called(changeNumber, fetchOptions)
	return args.Get(0).(*dtos.SplitChangesDTO), args.Error(1)
}

func ref[T any](t T) *T {
	return &t
}

type MSC struct {
	MySegments []dtos.MySegmentDTO `json:"mySegments"`
}

var _ service.SplitFetcher = (*splitFetcherMock)(nil)
