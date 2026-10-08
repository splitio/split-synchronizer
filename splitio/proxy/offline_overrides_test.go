package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	commonsflagsets "github.com/splitio/go-split-commons/v10/flagsets"
	inmemory "github.com/splitio/go-split-commons/v10/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type offlineFixture struct {
	db       persistent.DBWrapper
	splits   *storage.ProxySplitStorageImpl
	rbs      *storage.ProxyRuleBasedSegmentsStorageImpl
	segments *storage.ProxySegmentStorageImpl
	payload  []byte
}

func offlineTestFlag(name string, cn int64, sets ...string) dtos.SplitDTO {
	return dtos.SplitDTO{
		Name:             name,
		Status:           "ACTIVE",
		ChangeNumber:     cn,
		TrafficTypeName:  "user",
		DefaultTreatment: "off",
		Sets:             sets,
		Conditions: []dtos.ConditionDTO{{
			ConditionType: "ROLLOUT",
			Label:         "default rule",
			Partitions:    []dtos.PartitionDTO{{Treatment: "on", Size: 50}, {Treatment: "off", Size: 50}},
		}},
	}
}

// newOfflineFixture loads a snapshot holding flags (the last one at change number cn) the way startOffline does.
func newOfflineFixture(t *testing.T, cn int64, flags ...dtos.SplitDTO) *offlineFixture {
	t.Helper()
	logger := logging.NewLogger(nil)
	src, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "src.db"), nil)
	assert.Nil(t, err)
	storage.NewProxySplitStorage(src, logger, commonsflagsets.NewFlagSetFilter(nil), false).Update(flags, nil, cn)
	payload, err := src.GetRawSnapshot()
	assert.Nil(t, err)

	path, err := snapshot.WritePayloadToTmpFile(payload)
	assert.Nil(t, err)
	t.Cleanup(func() { os.Remove(path) })
	db, err := persistent.NewBoltWrapper(path, nil)
	assert.Nil(t, err)
	return &offlineFixture{
		db:       db,
		splits:   storage.NewProxySplitStorage(db, logger, commonsflagsets.NewFlagSetFilter(nil), true),
		rbs:      storage.NewProxyRuleBasedSegmentsStorage(db, logger, true),
		segments: storage.NewProxySegmentStorage(db, logger, true),
		payload:  payload,
	}
}

func writeOverridesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	assert.Nil(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

type capturedLogs struct {
	info, warning bytes.Buffer
}

func (c *capturedLogs) logger() logging.LoggerInterface {
	return logging.NewLogger(&logging.LoggerOptions{
		LogLevel:      logging.LevelInfo,
		InfoWriter:    &c.info,
		WarningWriter: &c.warning,
		ErrorWriter:   io.Discard,
	})
}

func fixedClock(ms int64) func() time.Time {
	return func() time.Time { return time.UnixMilli(ms) }
}

func TestSetupOverrides(t *testing.T) {
	fx := newOfflineFixture(t, 500, offlineTestFlag("f1", 400), offlineTestFlag("f2", 500))

	t.Run("no file configured leaves overrides off", func(t *testing.T) {
		o, stamp, err := setupOverrides(logging.NewLogger(nil), "", fx.splits, fixedClock(9999))
		assert.Nil(t, err)
		assert.Nil(t, o)
		assert.Zero(t, stamp)
	})

	t.Run("stamp is the clock when it is ahead of the snapshot", func(t *testing.T) {
		path := writeOverridesFile(t, "f1: \"on\"\n")
		o, stamp, err := setupOverrides(logging.NewLogger(nil), path, fx.splits, fixedClock(1790712345678))
		assert.Nil(t, err)
		assert.NotNil(t, o)
		assert.Equal(t, int64(1790712345678), stamp)
	})

	t.Run("stamp moves past the snapshot when the clock is behind it", func(t *testing.T) {
		path := writeOverridesFile(t, "f1: \"on\"\n")
		_, stamp, err := setupOverrides(logging.NewLogger(nil), path, fx.splits, fixedClock(100))
		assert.Nil(t, err)
		assert.Equal(t, int64(501), stamp)
	})

	t.Run("an empty file stamps with no overrides", func(t *testing.T) {
		o, stamp, err := setupOverrides(logging.NewLogger(nil), writeOverridesFile(t, ""), fx.splits, fixedClock(1790712345678))
		assert.Nil(t, err)
		if assert.NotNil(t, o) {
			assert.Empty(t, o.Entries)
		}
		assert.Equal(t, int64(1790712345678), stamp)
	})

	failures := map[string]string{
		"treatment the flag does not have": "f1: maybe\n",
		"malformed yaml":                   "f1: [oops\n",
		"unknown field":                    "f1:\n  treatment: \"on\"\n  foo: 1\n",
		"per-key targeting":                "f1:\n  treatment: \"on\"\n  keys: [a]\n",
	}
	for name, content := range failures {
		t.Run("fails startup on "+name, func(t *testing.T) {
			_, _, err := setupOverrides(logging.NewLogger(nil), writeOverridesFile(t, content), fx.splits, fixedClock(9999))
			assertInvalidConfiguration(t, err)
		})
	}

	t.Run("fails startup when the file is missing", func(t *testing.T) {
		_, _, err := setupOverrides(logging.NewLogger(nil), filepath.Join(t.TempDir(), "nope.yaml"), fx.splits, fixedClock(9999))
		assertInvalidConfiguration(t, err)
	})
}

func TestSetupOverridesLogs(t *testing.T) {
	fx := newOfflineFixture(t, 500, offlineTestFlag("new_checkout", 400), offlineTestFlag("search_rerank", 500))
	content := "new_checkout: \"off\"\nsearch_rerank:\n  treatment: \"on\"\n  config: '{\"model\":\"small\"}'\nold_flag: x\n"
	path := writeOverridesFile(t, content)

	var logs capturedLogs
	o, _, err := setupOverrides(logs.logger(), path, fx.splits, fixedClock(1790712345678))
	assert.Nil(t, err)

	info := logs.info.String()
	assert.Contains(t, info, fmt.Sprintf("Overrides loaded from %s (2 entries, sha256:%s); flag change number stamped at 1790712345678", path, o.SHA256))
	assert.Contains(t, info, `Override: new_checkout -> "off" for all keys (targeting ignored)`)
	assert.Contains(t, info, `Override: search_rerank -> "on" for all keys (targeting ignored), config replaced`)
	assert.NotContains(t, info, "old_flag")
	assert.Contains(t, logs.warning.String(), "Override for 'old_flag' skipped: flag not present in proxy data")
}

// Real offline storage, real controller: nothing may be served without the overlay and the Bolt data never changes.
func TestOverridesServedFromOfflineStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := newOfflineFixture(t, 500, offlineTestFlag("f1", 400, "set1"), offlineTestFlag("f2", 500, "set2"))
	logger := logging.NewLogger(nil)

	o, stamp, err := setupOverrides(logger, writeOverridesFile(t, "f1: \"on\"\n"), fx.splits, fixedClock(1000))
	assert.Nil(t, err)
	assert.Equal(t, int64(1000), stamp)

	controller := setupSdkController(&Options{
		Logger:                   logger,
		SplitFetcher:             offlineSplitFetcher{},
		ProxySplitStorage:        fx.splits,
		ProxySegmentStorage:      fx.segments,
		ProxyRBSegmentStorage:    fx.rbs,
		ProxyLargeSegmentStorage: inmemory.NewLargeSegmentsStorage(),
		SpecVersion:              "1.3",
		Offline:                  true,
		Overrides:                o,
		OverridesStamp:           stamp,
	})
	router := gin.New()
	controller.Register(router.Group("/api"))

	get := func(query string) dtos.RuleChangesDTO {
		req, _ := http.NewRequest(http.MethodGet, "/api/splitChanges?s=1.3&rbSince=-1&"+query, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, 200, resp.Code, resp.Body.String())
		var rules dtos.RuleChangesDTO
		assert.Nil(t, json.Unmarshal(resp.Body.Bytes(), &rules))
		return rules
	}
	labels := func(rules dtos.RuleChangesDTO) map[string]string {
		out := map[string]string{}
		for _, f := range rules.FeatureFlags.Splits {
			out[f.Name] = f.Conditions[0].Label
		}
		return out
	}

	for _, since := range []int64{-1, 0, 450, 500, 999} {
		rules := get(fmt.Sprintf("since=%d", since))
		assert.Equal(t, int64(1000), rules.FeatureFlags.Till, "since=%d", since)
		assert.Equal(t, map[string]string{"f1": "proxy override", "f2": "default rule"}, labels(rules), "since=%d", since)
	}

	// requested sets are honoured, with the flag overlaid
	for _, since := range []int64{-1, 450} {
		rules := get(fmt.Sprintf("since=%d&sets=set1", since))
		assert.Equal(t, int64(1000), rules.FeatureFlags.Till)
		assert.Equal(t, map[string]string{"f1": "proxy override"}, labels(rules), "since=%d with sets", since)
	}

	for _, since := range []int64{1000, 2000} {
		rules := get(fmt.Sprintf("since=%d", since))
		assert.Equal(t, since, rules.FeatureFlags.Till)
		assert.Empty(t, rules.FeatureFlags.Splits)
	}

	// overrides live only in memory
	reexported, err := fx.db.GetRawSnapshot()
	assert.Nil(t, err)
	assert.Equal(t, snapshot.PayloadChecksum(fx.payload), snapshot.PayloadChecksum(reexported))
}
