package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	psmocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/evaluator/impressionlabels"
	"github.com/splitio/go-split-commons/v10/engine/grammar"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v10/service"
	"github.com/splitio/go-split-commons/v10/service/api/specs"
	"github.com/splitio/go-split-commons/v10/service/mocks"
	cmnStorage "github.com/splitio/go-split-commons/v10/storage"
	inmemory "github.com/splitio/go-split-commons/v10/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
	loggingMocks "github.com/splitio/go-toolkit/v5/logging/mocks"

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
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_3,
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
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
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	splitFetcher.On("Fetch", ref(*service.MakeFlagRequestParams().WithChangeNumber(-1).WithChangeNumberRB(-1).WithSpecVersion(common.StringRef(specs.FLAG_V1_2)))).Return(
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
		&rbsStorage,
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

func TestSplitChangesOlderSinceOfflineServesSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(4), []string{"set1"}).
		Return((*dtos.SplitChangesDTO)(nil), storage.ErrSinceParamTooOld).
		Once()
	splitStorage.On("ChangesSince", int64(-1), []string{"set1"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 15, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}}}, nil).
		Once()
	splitStorage.On("ChangeNumber").Return(int64(20), nil).Once()

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(3)).
		Return((*dtos.RuleBasedSegmentsDTO)(nil), storage.ErrSinceParamTooOld).
		Once()
	rbsStorage.On("ChangesSince", int64(-1)).
		Return(&dtos.RuleBasedSegmentsDTO{Since: -1, Till: 9, RuleBasedSegments: []dtos.RuleBasedSegmentDTO{{Name: "r1"}}}, nil).
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
		&rbsStorage,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock,
		specs.FLAG_V1_2,
	)
	controller.ServeSnapshotWhenSinceTooOld()
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/splitChanges?since=4&rbSince=3&sets=set1&s=1.3", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, 200, resp.Code)
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	var rules dtos.RuleChangesDTO
	err = json.Unmarshal(body, &rules)
	assert.Nil(t, err)
	assert.Equal(t, int64(4), rules.FeatureFlags.Since)
	assert.Equal(t, int64(20), rules.FeatureFlags.Till)
	assert.Equal(t, 1, len(rules.FeatureFlags.Splits))
	assert.Equal(t, int64(3), rules.RuleBasedSegments.Since)
	assert.Equal(t, int64(9), rules.RuleBasedSegments.Till)
	assert.Equal(t, 1, len(rules.RuleBasedSegments.RuleBasedSegments))
	splitFetcher.AssertNotCalled(t, "Fetch", mock.Anything)
	splitStorage.AssertExpectations(t)
	rbsStorage.AssertExpectations(t)
}

func TestSplitChangesOlderSinceFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return((*dtos.SplitChangesDTO)(nil), storage.ErrSinceParamTooOld).
		Once()

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	splitFetcher := &mocks.MockSplitFetcher{}
	splitFetcher.On("Fetch", ref(*service.MakeFlagRequestParams().WithChangeNumber(-1).WithChangeNumberRB(-1).WithSpecVersion(common.StringRef(specs.FLAG_V1_2)))).
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
		&rbsStorage,
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
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

func TestSplitChangesFlagSetsUnorderedWarning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name        string
		query       string
		expectWarns int
	}{
		{"unordered", "sets=c,b,a", 1},
		{"duplicates", "sets=a,b,b", 1},
		{"ordered and unique", "sets=a,b,c", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var splitStorage psmocks.ProxySplitStorageMock
			splitStorage.On("ChangesSince", int64(-1), []string{"a", "b", "c"}).
				Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{}}, nil).Maybe()
			splitStorage.On("ChangesSince", int64(-1), []string{"a", "b"}).
				Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{}}, nil).Maybe()
			var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
			rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()
			var largeSegmentStorageMock largeSegmentStorageMock

			var warnings []string
			logger := &loggingMocks.MockLogger{
				ErrorCall:   func(msg ...interface{}) {},
				WarningCall: func(msg ...interface{}) { warnings = append(warnings, fmt.Sprint(msg...)) },
				InfoCall:    func(msg ...interface{}) {},
				DebugCall:   func(msg ...interface{}) {},
				VerboseCall: func(msg ...interface{}) {},
			}

			resp := httptest.NewRecorder()
			_, router := gin.CreateTestContext(resp)
			controller := NewSdkServerController(logger, &mocks.MockSplitFetcher{}, &splitStorage, nil, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
			controller.Register(router.Group("/api"))

			req, _ := http.NewRequest(http.MethodGet, "/api/splitChanges?since=-1&"+tc.query, nil)
			req.Header.Set("SplitSDKVersion", "go-1.1.1")
			router.ServeHTTP(resp, req)

			assert.Equal(t, 200, resp.Code)
			assert.Len(t, warnings, tc.expectWarns)
			for _, w := range warnings {
				assert.Contains(t, w, "SDK [go-1.1.1] is sending flagsets unordered or with duplicates.")
			}
			splitStorage.AssertNumberOfCalls(t, "ChangesSince", 1) // sanitized (sorted & deduped) sets are passed to storage
		})
	}
}

func TestSplitChangesWithFlagSetsStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var splitStorage psmocks.ProxySplitStorageMock
	splitStorage.On("ChangesSince", int64(-1), []string{"a", "c"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "s1", Status: "ACTIVE"}, {Name: "s2", Status: "ACTIVE"}}}, nil).
		Once()

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
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
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

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
		&rbsStorage,
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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

	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	ctx.Request.Header.Set("SplitSDKVersion", "go-1.1.1")
	ctx.Request.Header.Set("SplitSDKMachineIp", "1.2.3.4")
	ctx.Request.Header.Set("SplitSDKMachineName", "ip-1-2-3-4")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 500, resp.Code)
	assert.JSONEq(t, `{}`, resp.Body.String())
	assert.Equal(t, "{}", strings.TrimSpace(resp.Body.String())) // no second payload appended after the error
	assert.NotContains(t, resp.Body.String(), "mySegments")

	splitStorage.AssertExpectations(t)
	splitFetcher.AssertExpectations(t)
	segmentStorage.AssertExpectations(t)
}

func TestMySegmentsErrorIsNotCached(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "someKey").Return([]string(nil), errors.New("something")).Once()
	segmentStorage.On("SegmentsFor", "someKey").Return([]string{"segment1"}, nil).Once()

	var splitStorage psmocks.ProxySplitStorageMock
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	var largeSegmentStorageMock largeSegmentStorageMock

	resp := httptest.NewRecorder()
	_, router := gin.CreateTestContext(resp)
	router.Use(caching.MakeProxyCache().Handle)

	group := router.Group("/api")
	controller := NewSdkServerController(logging.NewLogger(nil), &mocks.MockSplitFetcher{}, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
	controller.Register(group)

	call := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/mySegments/someKey", nil)
		req.Header.Set("Authorization", "Bearer [redacted]")
		router.ServeHTTP(rec, req)
		return rec
	}

	// first call fails: the status must stay 500 (before the fix the handler also wrote a 200 payload,
	// which made the cache middleware store a corrupted `{}{"mySegments":[]}` entry)
	first := call()
	assert.Equal(t, 500, first.Code)
	assert.NotContains(t, first.Body.String(), "mySegments")

	// second call must hit the storage again (the error wasn't cached) and succeed
	second := call()
	assert.Equal(t, 200, second.Code)
	var ms MSC
	assert.Nil(t, json.Unmarshal(second.Body.Bytes(), &ms))
	assert.Equal(t, MSC{MySegments: []dtos.MySegmentDTO{{Name: "segment1"}}}, ms)

	// third call is served from the cache (storage mock only allows 2 calls)
	third := call()
	assert.Equal(t, 200, third.Code)
	assert.JSONEq(t, second.Body.String(), third.Body.String())

	segmentStorage.AssertExpectations(t)
	segmentStorage.AssertNumberOfCalls(t, "SegmentsFor", 2)
}

func TestMemberships(t *testing.T) {
	gin.SetMode(gin.TestMode)

	splitFetcher := &mocks.MockSplitFetcher{}
	var splitStorage psmocks.ProxySplitStorageMock
	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "keyTest").
		Return([]string{"segment1", "segment2"}, nil).
		Once()
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	var largeSegmentStorageMock largeSegmentStorageMock
	largeSegmentStorageMock.On("LargeSegmentsForUser", "keyTest").
		Return([]string{"largeSegment1", "largeSegment2"}).
		Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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

// Offline mode serves an empty large segment store: segments are returned, large segments never (G6).
func TestMembershipsOfflineHasNoLargeSegments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var segmentStorage psmocks.ProxySegmentStorageMock
	segmentStorage.On("SegmentsFor", "keyTest").Return([]string{"segment1"}, nil).Once()
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()
	var splitStorage psmocks.ProxySplitStorageMock

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	group := router.Group("/api")
	controller := NewSdkServerController(logging.NewLogger(nil), &mocks.MockSplitFetcher{}, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), inmemory.NewLargeSegmentsStorage(), specs.FLAG_V1_2)
	controller.Register(group)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/memberships/keyTest", nil)
	ctx.Request.Header.Set("Authorization", "Bearer someApiKey")
	router.ServeHTTP(resp, ctx.Request)
	assert.Equal(t, 200, resp.Code)

	var actual dtos.MembershipsResponseDTO
	assert.Nil(t, json.Unmarshal(resp.Body.Bytes(), &actual))
	assert.Equal(t, []dtos.Segment{{Name: "segment1"}}, actual.MySegments.Segments)
	assert.Empty(t, actual.MyLargeSegments.Segments)
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
	var rbsStorage psmocks.MockProxyRuleBasedSegmentStorage
	rbsStorage.On("ChangesSince", int64(-1)).Return(&dtos.RuleBasedSegmentsDTO{}).Once()

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	logger := logging.NewLogger(nil)

	group := router.Group("/api")
	controller := NewSdkServerController(logger, splitFetcher, &splitStorage, &segmentStorage, &rbsStorage, flagsets.NewMatcher(false, nil), &largeSegmentStorageMock, specs.FLAG_V1_2)
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
