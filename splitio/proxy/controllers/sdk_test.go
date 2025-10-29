package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	psmocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/engine/evaluator/impressionlabels"
	"github.com/splitio/go-split-commons/v8/engine/grammar"
	"github.com/splitio/go-split-commons/v8/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v8/service"
	"github.com/splitio/go-split-commons/v8/service/api/specs"
	"github.com/splitio/go-split-commons/v8/service/mocks"
	cmnStorage "github.com/splitio/go-split-commons/v8/storage"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSplitChangesImpressionsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE", ImpressionsDisabled: true}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	logger := logging.NewLogger(nil)
	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)
	assert.True(t, s.Splits[0].ImpressionsDisabled)
	assert.False(t, s.Splits[1].ImpressionsDisabled)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesRecentSince(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	logger := logging.NewLogger(nil)
	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	splitFetcher.On("Fetch", ref(*service.MakeFlagRequestParams().WithChangeNumber(-1).WithSpecVersion(common.StringRef(specs.FLAG_V1_2)))).Return(
		&dtos.FFResponseLegacy{
			SplitChanges: dtos.SplitChangesDTO{
				Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}},
			},
		}, nil).Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	splitFetcher.On("Fetch", ref(*service.MakeFlagRequestParams().WithChangeNumber(-1).WithSpecVersion(common.StringRef(specs.FLAG_V1_2)))).
		Return(nil, errors.New("something")).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
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

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1&sets=c,b,b,a", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(true, []string{"a", "c"}),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1&sets=c,b,b,a", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	assert.Nil(t, json.Unmarshal(body, &s))
	assert.Equal(t, 2, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesNewMatcherOldSpec(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{
			Since: -1,
			Till:  1,
			Splits: []dtos.SplitDTO{
				{
					Name:   "s1",
					Status: "ACTIVE",
					Conditions: []dtos.ConditionDTO{
						{
							MatcherGroup: dtos.MatcherGroupDTO{Matchers: []dtos.MatcherDTO{{MatcherType: constants.MatcherTypeEndsWith}}},
							Partitions:   []dtos.PartitionDTO{{Treatment: "on", Size: 100}},
							Label:        "some label",
						},
						{
							MatcherGroup: dtos.MatcherGroupDTO{Matchers: []dtos.MatcherDTO{{MatcherType: constants.MatcherTypeGreaterThanOrEqualToSemver}}},
							Partitions:   []dtos.PartitionDTO{{Treatment: "on", Size: 100}},
							Label:        "some label",
						},
					}},
			},
		}, nil).
		Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	logger := logging.NewLogger(nil)
	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	assert.Equal(t, 1, len(s.Splits[0].Conditions))
	cond := s.Splits[0].Conditions[0]
	assert.Equal(t, grammar.ConditionTypeWhitelist, cond.ConditionType)
	assert.Equal(t, constants.MatcherTypeAllKeys, cond.MatcherGroup.Matchers[0].MatcherType)
	assert.Equal(t, impressionlabels.UnsupportedMatcherType, cond.Label)
	assert.Equal(t, []dtos.PartitionDTO{{Treatment: "control", Size: 100}}, cond.Partitions)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSplitChangesNewMatcherNewSpec(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{
			Since: -1,
			Till:  1,
			Splits: []dtos.SplitDTO{
				{
					Name:   "s1",
					Status: "ACTIVE",
					Conditions: []dtos.ConditionDTO{
						{
							MatcherGroup: dtos.MatcherGroupDTO{Matchers: []dtos.MatcherDTO{{MatcherType: constants.MatcherTypeGreaterThanOrEqualToSemver}}},
							Partitions:   []dtos.PartitionDTO{{Treatment: "on", Size: 100}},
							Label:        "some label",
						},
					}},
			},
		}, nil).
		Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	logger := logging.NewLogger(nil)
	group := router.Group("/api")
	controller := NewSdkServerController(
		logger,
		splitFetcher,
		&splitStorage,
		nil,
		nil,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	q := ctx.Request.URL.Query()
	q.Add("s", specs.FLAG_V1_1)
	ctx.Request.URL.RawQuery = q.Encode()
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var s dtos.SplitChangesDTO
	err = json.Unmarshal(body, &s)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(s.Splits))
	assert.Equal(t, int64(-1), s.Since)
	assert.Equal(t, int64(1), s.Till)

	cond := s.Splits[0].Conditions[0]
	assert.Equal(t, constants.MatcherTypeGreaterThanOrEqualToSemver, cond.MatcherGroup.Matchers[0].MatcherType)
	assert.Equal(t, "some label", cond.Label)
	assert.Equal(t, []dtos.PartitionDTO{{Treatment: "on", Size: 100}}, cond.Partitions)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
}

func TestSegmentChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("ChangesSince", "someSegment", int64(-1)).
		Return(&dtos.SegmentChangesDTO{Name: "someSegment", Added: []string{"k1", "k2"}, Removed: []string{}, Since: -1, Till: 1}, nil).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/segmentChanges/someSegment?since=-1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("ChangesSince", "someSegment", int64(-1)).
		Return((*dtos.SegmentChangesDTO)(nil), storage.ErrSegmentNotFound).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "someKey").
		Return([]string{"segment1", "segment2"}, nil).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
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

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "someKey").
		Return([]string(nil), errors.New("something")).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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

func TestMemberships(t *testing.T) {
	gin.SetMode(gin.TestMode)

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "keyTest").
		Return([]string{"segment1", "segment2"}, nil).
		Once()

	var largeSegmentStorageMock largeSegmentStorageMock
	largeSegmentStorageMock.On("LargeSegmentsForUser", "keyTest").
		Return([]string{"largeSegment1", "largeSegment2"}).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/memberships/keyTest", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 200, resp.Code)

	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var actualDTO dtos.MembershipsResponseDTO
	err = json.Unmarshal(body, &actualDTO)
	assert.Nil(t, err)

	expectedDTO := dtos.MembershipsResponseDTO{
		MySegments: dtos.Memberships{
			Segments: []dtos.Segment{{Name: "segment1"}, {Name: "segment2"}},
		},
		MyLargeSegments: dtos.Memberships{
			Segments: []dtos.Segment{{Name: "largeSegment1"}, {Name: "largeSegment2"}},
		},
	}
	assert.Equal(t, expectedDTO, actualDTO)

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

func TestMembershipsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var largeSegmentStorageMock largeSegmentStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "keyTest").
		Return([]string{}, errors.New("error message.")).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, nil, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/memberships/keyTest", nil)
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

func ref[T any](t T) *T {
	return &t
}

type MSC struct {
	MySegments []dtos.MySegmentDTO `json:"mySegments"`
}

// --
type largeSegmentStorageMock struct {
	mock.Mock
}

func (s *largeSegmentStorageMock) SetChangeNumber(name string, till int64) {
	s.Called()
}
func (s *largeSegmentStorageMock) Update(name string, userKeys []string, till int64) {
}
func (s *largeSegmentStorageMock) ChangeNumber(name string) int64 {
	return s.Called(name).Get(0).(int64)
}
func (s *largeSegmentStorageMock) Count() int {
	return s.Called().Get(0).(int)
}
func (s *largeSegmentStorageMock) LargeSegmentsForUser(userKey string) []string {
	return s.Called(userKey).Get(0).([]string)
}
func (s *largeSegmentStorageMock) IsInLargeSegment(name string, key string) (bool, error) {
	args := s.Called(name, key)
	return args.Get(0).(bool), args.Error(1)
}
func (s *largeSegmentStorageMock) TotalKeys(name string) int {
	return s.Called(name).Get(0).(int)
}

// --

var _ cmnStorage.LargeSegmentsStorage = (*largeSegmentStorageMock)(nil)
