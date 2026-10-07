package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/overrides"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	psmocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/service/api/specs"
	"github.com/splitio/go-split-commons/v10/service/mocks"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	testStamp      = int64(1000)
	testSnapshotCN = int64(500)
)

func loadTestOverrides(t *testing.T, yaml string) *overrides.Overrides {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	assert.Nil(t, os.WriteFile(path, []byte(yaml), 0644))
	o, err := overrides.Load(path)
	assert.Nil(t, err)
	return o
}

func plainFlag(name string) dtos.SplitDTO {
	return dtos.SplitDTO{
		Name:              name,
		Status:            "ACTIVE",
		TrafficTypeName:   "user",
		TrafficAllocation: 50,
		DefaultTreatment:  "off",
		Killed:            true,
		Conditions: []dtos.ConditionDTO{{
			ConditionType: "ROLLOUT",
			Label:         "default rule",
			Partitions:    []dtos.PartitionDTO{{Treatment: "on", Size: 50}, {Treatment: "off", Size: 50}},
		}},
	}
}

type overrideHarness struct {
	splits *psmocks.ProxySplitStorageMock
	rbs    *psmocks.MockProxyRuleBasedSegmentStorage
	fetch  *mocks.MockSplitFetcher
	router *gin.Engine
}

// newOverrideHarness serves f1 (overridden to "on") and f2 (untouched) from a snapshot at testSnapshotCN, with the
// startup stamp at testStamp.
func newOverrideHarness(t *testing.T, o *overrides.Overrides) *overrideHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &overrideHarness{
		splits: &psmocks.ProxySplitStorageMock{},
		rbs:    &psmocks.MockProxyRuleBasedSegmentStorage{},
		fetch:  &mocks.MockSplitFetcher{},
	}
	// the full set, as the offline storage hands it out for -1
	h.splits.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: testSnapshotCN, Splits: []dtos.SplitDTO{plainFlag("f1"), plainFlag("f2")}}, nil).Maybe()
	h.splits.On("ChangesSince", int64(-1), []string{"set1"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: testSnapshotCN, Splits: []dtos.SplitDTO{plainFlag("f1")}}, nil).Maybe()
	h.splits.On("ChangesSince", int64(-1), []string{"set1", "set2"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: testSnapshotCN, Splits: []dtos.SplitDTO{plainFlag("f2")}}, nil).Maybe()

	// rule-based segments: a current rbSince answers from the window, an old one with the full set
	h.rbs.On("ChangesSince", int64(9)).
		Return(&dtos.RuleBasedSegmentsDTO{Since: 9, Till: 9, RuleBasedSegments: []dtos.RuleBasedSegmentDTO{}}, nil).Maybe()
	h.rbs.On("ChangesSince", int64(3)).
		Return((*dtos.RuleBasedSegmentsDTO)(nil), storage.ErrSinceParamTooOld).Maybe()
	h.rbs.On("ChangesSince", int64(-1)).
		Return(&dtos.RuleBasedSegmentsDTO{Since: -1, Till: 9, RuleBasedSegments: []dtos.RuleBasedSegmentDTO{{Name: "r1"}}}, nil).Maybe()

	router := gin.New()
	controller := NewSdkServerController(
		logging.NewLogger(nil),
		h.fetch,
		h.splits,
		nil,
		h.rbs,
		flagsets.NewMatcher(false, nil),
		&largeSegmentStorageMock{},
		specs.FLAG_V1_3,
	)
	controller.ServeSnapshotWhenSinceTooOld()
	controller.SetOverrides(o, testStamp)
	controller.Register(router.Group("/api"))
	h.router = router
	return h
}

func (h *overrideHarness) get(t *testing.T, query string) (dtos.RuleChangesDTO, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "/api/splitChanges?s=1.3&"+query, nil)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	assert.Equal(t, 200, resp.Code)
	body, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	var rules dtos.RuleChangesDTO
	assert.Nil(t, json.Unmarshal(body, &rules))
	return rules, string(body)
}

func flagsByName(splits []dtos.SplitDTO) map[string]dtos.SplitDTO {
	out := map[string]dtos.SplitDTO{}
	for _, s := range splits {
		out[s.Name] = s
	}
	return out
}

func assertOverridden(t *testing.T, f dtos.SplitDTO) {
	t.Helper()
	if assert.Len(t, f.Conditions, 1) {
		assert.Equal(t, "proxy override", f.Conditions[0].Label)
		assert.Equal(t, []dtos.PartitionDTO{{Treatment: "on", Size: 100}}, f.Conditions[0].Partitions)
	}
	assert.Equal(t, 100, f.TrafficAllocation)
	assert.Equal(t, "on", f.DefaultTreatment)
	assert.False(t, f.Killed)
}

func assertUntouched(t *testing.T, f dtos.SplitDTO) {
	t.Helper()
	assert.Equal(t, plainFlag(f.Name), f)
}

func TestOverriddenSplitChangesBelowStampServesFullOverlaidSet(t *testing.T) {
	// -1 is answered by storage directly in offline mode (it never reports "too old"), so it must be caught here.
	for _, since := range []int64{-1, 0, 499, 500, 999} {
		t.Run(fmt.Sprintf("since=%d", since), func(t *testing.T) {
			h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
			rules, _ := h.get(t, fmt.Sprintf("since=%d&rbSince=-1", since))

			assert.Equal(t, since, rules.FeatureFlags.Since)
			assert.Equal(t, testStamp, rules.FeatureFlags.Till)
			flags := flagsByName(rules.FeatureFlags.Splits)
			if assert.Len(t, flags, 2) {
				assertOverridden(t, flags["f1"])
				assertUntouched(t, flags["f2"])
			}
			// flags never go through the windowed lookup, whatever the since
			h.splits.AssertNumberOfCalls(t, "ChangesSince", 1)
			h.splits.AssertCalled(t, "ChangesSince", int64(-1), []string(nil))
			h.fetch.AssertNotCalled(t, "Fetch", mock.Anything)
		})
	}
}

func TestOverriddenSplitChangesAtOrAboveStampIsEmpty(t *testing.T) {
	for _, since := range []int64{1000, 1001, 1500} {
		t.Run(fmt.Sprintf("since=%d", since), func(t *testing.T) {
			h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
			rules, body := h.get(t, fmt.Sprintf("since=%d&rbSince=9", since))

			assert.Equal(t, since, rules.FeatureFlags.Since)
			assert.Equal(t, since, rules.FeatureFlags.Till, "till never goes below the since the SDK sent")
			assert.Empty(t, rules.FeatureFlags.Splits)
			assert.Contains(t, body, fmt.Sprintf(`"ff":{"s":%d,"t":%d,"d":[]}`, since, since), "an empty list, not null")
			h.splits.AssertNotCalled(t, "ChangesSince", mock.Anything, mock.Anything)
			h.fetch.AssertNotCalled(t, "Fetch", mock.Anything)
		})
	}
}

func TestOverriddenSplitChangesRespectsRequestedSets(t *testing.T) {
	t.Run("below the stamp only the matching flags are returned", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		for _, since := range []int64{-1, 700} {
			rules, _ := h.get(t, fmt.Sprintf("since=%d&rbSince=-1&sets=set1", since))
			assert.Equal(t, testStamp, rules.FeatureFlags.Till)
			flags := flagsByName(rules.FeatureFlags.Splits)
			if assert.Len(t, flags, 1) {
				assertOverridden(t, flags["f1"])
			}
		}
		h.splits.AssertNotCalled(t, "ChangesSince", int64(700), mock.Anything)
	})

	t.Run("a flag filtered out by the sets is not returned even if overridden", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		rules, _ := h.get(t, "since=-1&rbSince=-1&sets=set1,set2")
		flags := flagsByName(rules.FeatureFlags.Splits)
		if assert.Len(t, flags, 1) {
			assertUntouched(t, flags["f2"])
		}
	})

	t.Run("at or above the stamp the response stays empty", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		rules, _ := h.get(t, "since=1000&rbSince=-1&sets=set1")
		assert.Empty(t, rules.FeatureFlags.Splits)
		assert.Equal(t, int64(1000), rules.FeatureFlags.Till)
	})
}

func TestOverriddenRuleBasedSegmentsKeepTheirOwnWindow(t *testing.T) {
	t.Run("old since with a current rbSince", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		rules, _ := h.get(t, "since=-1&rbSince=9")
		assert.Equal(t, testStamp, rules.FeatureFlags.Till)
		assert.Len(t, rules.FeatureFlags.Splits, 2)
		assert.Equal(t, int64(9), rules.RuleBasedSegments.Till)
		assert.Empty(t, rules.RuleBasedSegments.RuleBasedSegments)
	})

	t.Run("current since with an old rbSince gets the full rule-based set", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		rules, _ := h.get(t, "since=1000&rbSince=3")
		assert.Empty(t, rules.FeatureFlags.Splits)
		assert.Equal(t, int64(1000), rules.FeatureFlags.Till)
		assert.Equal(t, int64(3), rules.RuleBasedSegments.Since)
		assert.Equal(t, int64(9), rules.RuleBasedSegments.Till)
		assert.Len(t, rules.RuleBasedSegments.RuleBasedSegments, 1)
	})

	t.Run("old since with an old rbSince", func(t *testing.T) {
		h := newOverrideHarness(t, loadTestOverrides(t, "f1: \"on\"\n"))
		rules, _ := h.get(t, "since=0&rbSince=3")
		assert.Equal(t, testStamp, rules.FeatureFlags.Till)
		assert.Len(t, rules.RuleBasedSegments.RuleBasedSegments, 1)
	})
}

func TestOverriddenSplitChangesWithAnEmptyFileStillStamps(t *testing.T) {
	h := newOverrideHarness(t, loadTestOverrides(t, ""))

	rules, _ := h.get(t, "since=700&rbSince=-1")
	assert.Equal(t, testStamp, rules.FeatureFlags.Till)
	flags := flagsByName(rules.FeatureFlags.Splits)
	if assert.Len(t, flags, 2) {
		assertUntouched(t, flags["f1"])
		assertUntouched(t, flags["f2"])
	}

	rules, _ = h.get(t, "since=1000&rbSince=-1")
	assert.Empty(t, rules.FeatureFlags.Splits)
	assert.Equal(t, int64(1000), rules.FeatureFlags.Till)
}
