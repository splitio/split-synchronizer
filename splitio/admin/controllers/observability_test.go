package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
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
		t.Error("hay crap.")
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
