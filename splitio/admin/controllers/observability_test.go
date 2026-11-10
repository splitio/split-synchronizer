package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
)

func TestSyncObservabilityEndpoint(t *testing.T) {
	logger := logging.NewLogger(nil)

	extSplitStorage := &extMockSplitStorage{
		&mocks.MockSplitStorage{
			SplitNamesCall: func() []string {
				return []string{"split1", "split2", "split3"}
			},
			SegmentNamesCall: func() *set.ThreadUnsafeSet {
				return set.NewSet("segment1")
			},
			GetAllFlagSetNamesCall: func() []string {
				return []string{"fSet1", "fSet2"}
			},
		},
		nil,
	}

	extSegmentStorage := &extMockSegmentStorage{
		MockSegmentStorage: &mocks.MockSegmentStorage{},
		SizeCall: func(name string) (int, error) {
			switch name {
			case "segment1":
				return 10, nil
			case "segment2":
				return 20, nil
			}
			return 0, nil
		},
	}

	oSplitStorage, err := observability.NewObservableSplitStorage(extSplitStorage, logger)
	if err != nil {
		t.Error(err)
		return
	}

	oSegmentStorage, err := observability.NewObservableSegmentStorage(logger, extSplitStorage, extSegmentStorage)
	if err != nil {
		t.Error(err)
		return
	}

	storages := adminCommon.Storages{
		SplitStorage:   oSplitStorage,
		SegmentStorage: oSegmentStorage,
	}

	ctrl, err := NewObservabilityController(false, logger, storages)

	if err != nil {
		t.Error(err)
		return
	}

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/observability", nil)
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Errorf("Something went wrong. Status Code %v", resp.Code)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	var result ObservabilityDto
	if err := json.Unmarshal(body, &result); err != nil {
		t.Error("there should be no error ", err)
	}

	if len(result.ActiveFlagSets) != 2 {
		t.Errorf("Active flag sets should be 2. Actual %d", len(result.ActiveFlagSets))
	}

	if len(result.ActiveSplits) != 3 {
		t.Errorf("Active splits should be 3. Actual %d", len(result.ActiveSplits))
	}

	if len(result.ActiveSegments) != 1 {
		t.Errorf("Active segments should be 1. Actual %d", len(result.ActiveSegments))
	}
}

// TODO: should unify this classes
type extMockSplitStorage struct {
	*mocks.MockSplitStorage
	UpdateWithErrorsCall func([]dtos.SplitDTO, []dtos.SplitDTO, int64) error
}

func (e *extMockSplitStorage) UpdateWithErrors(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) error {
	return e.UpdateWithErrorsCall(toAdd, toRemove, cn)
}

type extMockSegmentStorage struct {
	*mocks.MockSegmentStorage
	UpdateWithSummaryCall func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error)
	SizeCall              func(string) (int, error)
}

func (e *extMockSegmentStorage) UpdateWithSummary(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, till int64) (added int, removed int, err error) {
	return e.UpdateWithSummaryCall(name, toAdd, toRemove, till)
}

func (e *extMockSegmentStorage) Size(name string) (int, error) {
	return e.SizeCall(name)
}

func TestProxyObservabilityEndpoint(t *testing.T) {
	logger := logging.NewLogger(nil)
	extSplitStorage := &extMockSplitStorage{
		&mocks.MockSplitStorage{
			SplitNamesCall: func() []string {
				return []string{"split1", "split2", "split3"}
			},
			SegmentNamesCall: func() *set.ThreadUnsafeSet {
				return set.NewSet("segment1")
			},
			GetAllFlagSetNamesCall: func() []string {
				return []string{"fSet1", "fSet2"}
			},
		},
		nil,
	}

	extSegmentStorage := &extMockSegmentStorage{
		MockSegmentStorage: &mocks.MockSegmentStorage{},
		SizeCall: func(name string) (int, error) {
			switch name {
			case "segment1":
				return 10, nil
			case "segment2":
				return 20, nil
			}
			return 0, nil
		},
	}

	oSplitStorage, err := observability.NewObservableSplitStorage(extSplitStorage, logger)
	if err != nil {
		t.Error(err)
		return
	}

	oSegmentStorage, err := observability.NewObservableSegmentStorage(logger, extSplitStorage, extSegmentStorage)
	if err != nil {
		t.Error(err)
		return
	}

	localTelemetryStorage := storage.NewTimeslicedProxyEndpointTelemetry(
		storage.NewProxyTelemetryFacade(),
		50,
		5,
	)

	storages := adminCommon.Storages{
		SplitStorage:          oSplitStorage,
		SegmentStorage:        oSegmentStorage,
		LocalTelemetryStorage: localTelemetryStorage,
	}

	ctrl, err := NewObservabilityController(true, logger, storages)
	if err != nil {
		t.Error(err)
		return
	}

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/observability", nil)
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Errorf("Something went wrong. Status Code %v", resp.Code)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	var result ObservabilityDto
	if err := json.Unmarshal(body, &result); err != nil {
		t.Error("there should be no error ", err)
	}

	if len(result.ActiveFlagSets) != 2 {
		t.Errorf("Active flag sets should be 2. Actual %d", len(result.ActiveFlagSets))
	}

	if len(result.ActiveSplits) != 3 {
		t.Errorf("Active splits should be 3. Actual %d", len(result.ActiveSplits))
	}

	if len(result.ActiveSegments) != 1 {
		t.Errorf("Active segments should be 1. Actual %d", len(result.ActiveSegments))
	}
}

type fakeRedisStateReporter struct {
	status redisstate.Status
}

func (f *fakeRedisStateReporter) Status() redisstate.Status {
	return f.status
}

func proxyObservabilityBody(t *testing.T, redisState redisstate.StatusReporter) map[string]interface{} {
	t.Helper()
	logger := logging.NewLogger(nil)
	extSplitStorage := &extMockSplitStorage{
		&mocks.MockSplitStorage{
			SplitNamesCall: func() []string { return []string{"split1"} },
			SegmentNamesCall: func() *set.ThreadUnsafeSet {
				return set.NewSet("segment1")
			},
			GetAllFlagSetNamesCall: func() []string { return []string{"fSet1"} },
		},
		nil,
	}
	extSegmentStorage := &extMockSegmentStorage{
		MockSegmentStorage: &mocks.MockSegmentStorage{},
		SizeCall:           func(name string) (int, error) { return 0, nil },
	}

	oSplitStorage, err := observability.NewObservableSplitStorage(extSplitStorage, logger)
	if err != nil {
		t.Fatal(err)
	}
	oSegmentStorage, err := observability.NewObservableSegmentStorage(logger, extSplitStorage, extSegmentStorage)
	if err != nil {
		t.Fatal(err)
	}

	storages := adminCommon.Storages{
		SplitStorage:          oSplitStorage,
		SegmentStorage:        oSegmentStorage,
		LocalTelemetryStorage: storage.NewTimeslicedProxyEndpointTelemetry(storage.NewProxyTelemetryFacade(), 50, 5),
		RedisState:            redisState,
	}

	ctrl, err := NewObservabilityController(true, logger, storages)
	if err != nil {
		t.Fatal(err)
	}

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)
	ctx.Request, _ = http.NewRequest(http.MethodGet, "/observability", nil)
	router.ServeHTTP(resp, ctx.Request)

	if resp.Code != 200 {
		t.Fatalf("Status Code %v", resp.Code)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProxyObservabilityIncludesRedisState(t *testing.T) {
	reporter := &fakeRedisStateReporter{status: redisstate.Status{Healthy: true, PendingChanges: 7, LagMs: 42}}
	result := proxyObservabilityBody(t, reporter)

	raw, ok := result["redisState"]
	if !ok {
		t.Fatal("redisState should be present when the proxy writes to Redis")
	}
	state, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("redisState should be an object. Actual %T", raw)
	}
	if state["healthy"] != true {
		t.Errorf("redisState.healthy should be true. Actual %v", state["healthy"])
	}
	if state["pendingChanges"] != float64(7) {
		t.Errorf("redisState.pendingChanges should be 7. Actual %v", state["pendingChanges"])
	}
	if state["lagMs"] != float64(42) {
		t.Errorf("redisState.lagMs should be 42. Actual %v", state["lagMs"])
	}
	if _, ok := result["proxyEndpointStats"]; !ok {
		t.Error("proxyEndpointStats should still be present")
	}
}

func TestProxyObservabilityWithoutRedisStateUnchanged(t *testing.T) {
	result := proxyObservabilityBody(t, nil)

	if _, ok := result["redisState"]; ok {
		t.Error("redisState should be absent when the proxy does not write to Redis")
	}
	for _, key := range []string{"activeSplits", "activeSegments", "activeFlagSets", "proxyEndpointStats", "proxyEndpointStatsTotal"} {
		if _, ok := result[key]; !ok {
			t.Errorf("key %s should be present", key)
		}
	}
	if len(result) != 5 {
		t.Errorf("expected exactly 5 keys. Actual %d", len(result))
	}
}
