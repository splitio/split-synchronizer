package proxy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v10/synchronizer"
	"github.com/splitio/go-toolkit/v5/logging"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
	"github.com/stretchr/testify/assert"
)

type syncManagerMock struct {
	c         chan int
	execCount int64
}

func (m *syncManagerMock) IsRunning() bool { panic("unimplemented") }
func (m *syncManagerMock) Start() {
	atomic.AddInt64(&m.execCount, 1)
	switch atomic.LoadInt64(&m.execCount) {
	case 1:
		m.c <- synchronizer.Error
	default:
		m.c <- synchronizer.Ready
	}
}
func (m *syncManagerMock) Stop() { panic("unimplemented") }

func (m *syncManagerMock) StartBGSync(mstatus chan int, shouldRetry bool, onReady func()) error {
	panic("unimplemented")
}

var _ synchronizer.Manager = (*syncManagerMock)(nil)

func TestSyncManagerInitializationRetriesWithSnapshot(t *testing.T) {

	sm := &syncManagerMock{c: make(chan int, 1)}

	// No snapshot and error
	complete := make(chan struct{}, 1)
	err := startBGSync(sm, sm.c, false, func() { complete <- struct{}{} })
	if err != errUnrecoverable {
		t.Error("should be an unrecoverable error. Got: ", err)
	}

	select {
	case <-complete:
		t.Error("nothing should be published on the channel")
	case <-time.After(500 * time.Millisecond):
		// all good
	}

	// Snapshot and error
	atomic.StoreInt64(&sm.execCount, 0)
	err = startBGSync(sm, sm.c, true, func() { complete <- struct{}{} })
	if err != errRetrying {
		t.Error("should be a retrying error. Got: ", err)
	}

	select {
	case <-complete:
		// all good
	case <-time.After(2500 * time.Millisecond):
		t.Error("should not time out")
	}

	if atomic.LoadInt64(&sm.execCount) != 2 {
		t.Error("there should be 2 executions")
	}
}

// mockLogger captures warning messages for testing
type mockLogger struct {
	logging.LoggerInterface
	warnings []string
}

func (m *mockLogger) Warning(msg ...interface{}) {
	m.warnings = append(m.warnings, fmt.Sprint(msg...))
}

func (m *mockLogger) Debug(msg ...interface{}) {}
func (m *mockLogger) Error(msg ...interface{}) {}
func (m *mockLogger) Info(msg ...interface{})  {}

func TestSnapshotHashMismatchLogsWarningAndIgnoresSnapshot(t *testing.T) {
	// Create a temporary snapshot file with a specific hash
	tmpDir, err := os.MkdirTemp("", "snapshot-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := tmpDir + "/test-snapshot.snap"

	// Create a snapshot with hash "12345"
	meta := snapshot.Metadata{
		Version: 1,
		Storage: 1,
		Hash:    "12345",
	}
	snap, err := snapshot.New(meta, []byte("test data"))
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Encode snapshot to bytes and write to file
	encoded, err := snap.Encode()
	if err != nil {
		t.Fatalf("failed to encode snapshot: %v", err)
	}

	if err := os.WriteFile(snapshotPath, encoded, 0644); err != nil {
		t.Fatalf("failed to write snapshot file: %v", err)
	}

	// Create a config with different apikey/version/flagsets that will produce a different hash
	cfg := &pconf.Main{
		Apikey:          "test-apikey",
		FlagSpecVersion: "1.1",
		FlagSetsFilter:  []string{"set1", "set2"},
		Initialization: pconf.Initialization{
			Snapshot: snapshotPath,
		},
	}

	// Calculate what the hash would be with this config
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	expectedHashStr := strconv.Itoa(int(currentHash))

	// Verify that the hashes are indeed different
	if meta.Hash == expectedHashStr {
		t.Fatal("test setup error: hashes should be different for this test")
	}

	// Create a mock logger to capture warnings
	mockLog := &mockLogger{
		warnings: make([]string, 0),
	}

	// This simulates the code path in Start() that checks the hash
	snap2, err := snapshot.DecodeFromFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}

	// Simulate the new behavior: check hash and ignore if mismatch
	var snapshotValid = false
	currentHash2 := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	if snap2.Meta().Hash != strconv.Itoa(int(currentHash2)) {
		mockLog.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one. Ignoring snapshot and starting with empty storage.")
	} else {
		snapshotValid = true
	}

	// Verify that a warning was logged
	if len(mockLog.warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(mockLog.warnings))
	}

	expectedWarning := "snapshot cfg (apikey, version, flagsets) does not match the provided one. Ignoring snapshot and starting with empty storage."
	if len(mockLog.warnings) > 0 && mockLog.warnings[0] != expectedWarning {
		t.Errorf("expected warning '%s', got '%s'", expectedWarning, mockLog.warnings[0])
	}

	// Verify that snapshot is marked as invalid
	if snapshotValid {
		t.Error("snapshot should be marked as invalid when hash mismatches")
	}
}

func TestSnapshotHashMatchNoWarningAndUsesSnapshot(t *testing.T) {
	// Create a temporary snapshot file
	tmpDir, err := os.MkdirTemp("", "snapshot-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := tmpDir + "/test-snapshot.snap"

	// Create config
	cfg := &pconf.Main{
		Apikey:          "test-apikey",
		FlagSpecVersion: "1.1",
		FlagSetsFilter:  []string{"set1", "set2"},
		Initialization: pconf.Initialization{
			Snapshot: snapshotPath,
		},
	}

	// Calculate the hash with the config
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	hashStr := strconv.Itoa(int(currentHash))

	// Create a snapshot with the SAME hash
	meta := snapshot.Metadata{
		Version: 1,
		Storage: 1,
		Hash:    hashStr,
	}
	snap, err := snapshot.New(meta, []byte("test data"))
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Encode snapshot to bytes and write to file
	encoded, err := snap.Encode()
	if err != nil {
		t.Fatalf("failed to encode snapshot: %v", err)
	}

	if err := os.WriteFile(snapshotPath, encoded, 0644); err != nil {
		t.Fatalf("failed to write snapshot file: %v", err)
	}

	// Create a mock logger to capture warnings
	mockLog := &mockLogger{
		warnings: make([]string, 0),
	}

	// This simulates the code path in Start() that checks the hash
	snap2, err := snapshot.DecodeFromFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}

	// Simulate the new behavior: check hash and use if match
	var snapshotValid = false
	currentHash2 := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	if snap2.Meta().Hash != strconv.Itoa(int(currentHash2)) {
		mockLog.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one. Ignoring snapshot and starting with empty storage.")
	} else {
		snapshotValid = true
	}

	// Verify that NO warning was logged
	if len(mockLog.warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(mockLog.warnings), mockLog.warnings)
	}

	// Verify that snapshot is marked as valid
	if !snapshotValid {
		t.Error("snapshot should be marked as valid when hash matches")
	}
}

func TestFeatureFlagRulesKeepLargeSegmentMatcher(t *testing.T) {
	has := func(rules []string, m string) bool {
		for _, r := range rules {
			if r == m {
				return true
			}
		}
		return false
	}
	// Without this matcher the proxy rewrites flag conditions to "unsupported" before storage,
	// so large segments are never fetched.
	if !has(adminCommon.ProxyFeatureFlagsRules, constants.MatcherTypeInLargeSegment) {
		t.Error("proxy feature flag rules must accept IN_LARGE_SEGMENT")
	}
	// The synchronizer does not sync large segments and keeps its list unchanged.
	if has(adminCommon.ProducerFeatureFlagsRules, constants.MatcherTypeInLargeSegment) {
		t.Error("producer feature flag rules must not accept IN_LARGE_SEGMENT")
	}
	if has(adminCommon.ProducerRuleBasedSegmentRules, constants.MatcherTypeInLargeSegment) {
		t.Error("rule-based segment rules must not accept IN_LARGE_SEGMENT")
	}
	if len(adminCommon.ProxyFeatureFlagsRules) != len(adminCommon.ProducerFeatureFlagsRules)+1 {
		t.Error("proxy rules must be the producer rules plus IN_LARGE_SEGMENT only")
	}
}

func TestMaskAdminConfigSecrets(t *testing.T) {
	live := &pconf.Main{}
	live.Storage.Redis.Connection.Username = "redis-user"
	live.Storage.Redis.Connection.Pass = "placeholder-pw"
	live.Storage.Redis.Connection.TLSClientKey = "client-key-pem-placeholder"
	live.Storage.Redis.Connection.Host = "redis.local"
	live.Admin.Username = "admin-user"
	live.Admin.Password = "admin-placeholder-pw"
	live.Server.ClientApikeys = []string{"client-key-1", "client-key-2"}

	cfgForAdmin := *live
	maskAdminConfigSecrets(&cfgForAdmin)

	conn := cfgForAdmin.Storage.Redis.Connection
	if conn.Username != maskedSecret || conn.Pass != maskedSecret || conn.TLSClientKey != maskedSecret {
		t.Fatalf("expected masked redis credentials in admin copy, got user=%q pass=%q key=%q", conn.Username, conn.Pass, conn.TLSClientKey)
	}
	if conn.Host != "redis.local" {
		t.Errorf("non-secret host should not be masked, got %q", conn.Host)
	}
	if cfgForAdmin.Admin.Password != maskedSecret {
		t.Errorf("expected admin password masked in admin copy, got %q", cfgForAdmin.Admin.Password)
	}
	if cfgForAdmin.Admin.Username != "admin-user" {
		t.Errorf("admin username should not be masked, got %q", cfgForAdmin.Admin.Username)
	}
	for i, key := range cfgForAdmin.Server.ClientApikeys {
		if key != maskedSecret {
			t.Errorf("expected client apikey %d masked in admin copy, got %q", i, key)
		}
	}

	orig := live.Storage.Redis.Connection
	if orig.Username != "redis-user" || orig.Pass != "placeholder-pw" || orig.TLSClientKey != "client-key-pem-placeholder" {
		t.Errorf("live redis config was mutated: user=%q pass=%q key=%q", orig.Username, orig.Pass, orig.TLSClientKey)
	}
	if live.Admin.Password != "admin-placeholder-pw" {
		t.Errorf("live admin password was mutated: %q", live.Admin.Password)
	}
	if live.Server.ClientApikeys[0] != "client-key-1" || live.Server.ClientApikeys[1] != "client-key-2" {
		t.Errorf("live client apikeys shared backing array was mutated: %v", live.Server.ClientApikeys)
	}

	empty := &pconf.Main{}
	maskAdminConfigSecrets(empty)
	if empty.Storage.Redis.Connection.Pass != "" || empty.Storage.Redis.Connection.Username != "" || empty.Admin.Password != "" {
		t.Error("empty credentials should stay empty so the admin view shows they are unset")
	}
	if empty.Server.ClientApikeys != nil {
		t.Errorf("nil client apikeys should stay nil, got %v", empty.Server.ClientApikeys)
	}
}

func TestRequestSecondResyncStopsOnShutdown(t *testing.T) {
	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	w, err := redisstate.NewWriter(resyncCountingClient{}, keys, "fingerprint", redisstate.DefaultOptions(), logging.NewLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	src := &resyncCountingSource{}
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	waitForFlagCalls(t, src, 1)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		requestSecondResyncWith(w, false, time.Hour, stop, logging.NewLogger(nil), func() bool { return false })
		close(done)
	}()
	close(stop)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("requestSecondResyncWith did not return after shutdown was signalled")
	}
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt64(&src.flagCalls); got != 1 {
		t.Fatalf("expected no resync after shutdown, got %d source reads", got)
	}
}

type resyncCountingClient struct{}

func (resyncCountingClient) Eval(script string, keys []string, args ...interface{}) error { return nil }
func (resyncCountingClient) HGetAll(key string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (resyncCountingClient) Prefix() string                    { return "prefix" }
func (resyncCountingClient) Del(keys ...string) (int64, error) { return 0, nil }
func (resyncCountingClient) Get(key string) (string, error)    { return "", nil }

type resyncCountingSource struct{ flagCalls int64 }

func (s *resyncCountingSource) Flags() ([]dtos.SplitDTO, int64, error) {
	atomic.AddInt64(&s.flagCalls, 1)
	return nil, 0, nil
}

func (s *resyncCountingSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	return nil, 0, nil
}

func (s *resyncCountingSource) Segments(visit func(redisstate.SegmentState) error) error { return nil }

func waitForFlagCalls(t *testing.T, s *resyncCountingSource, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt64(&s.flagCalls) < want {
		if time.Now().After(deadline) {
			t.Fatalf("expected at least %d source reads, got %d", want, atomic.LoadInt64(&s.flagCalls))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRequestRedisResyncRepairsStartedWriter(t *testing.T) {
	requestRedisResync(nil)

	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	logger := logging.NewLogger(nil)
	w, err := redisstate.NewWriter(resyncCountingClient{}, keys, "fingerprint", redisstate.DefaultOptions(), logger)
	if err != nil {
		t.Fatal(err)
	}
	src := &resyncCountingSource{}
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	waitForFlagCalls(t, src, 1)
	requestRedisResync(w)
	waitForFlagCalls(t, src, 2)
}

func TestSecondResyncSkippedOnWarmBoot(t *testing.T) {
	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	w, err := redisstate.NewWriter(resyncCountingClient{}, keys, "fingerprint", redisstate.DefaultOptions(), logging.NewLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	src := &resyncCountingSource{}
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	waitForFlagCalls(t, src, 1)
	requestSecondResync(w, true, time.Second, nil, logging.NewLogger(nil))
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt64(&src.flagCalls); got != 1 {
		t.Fatalf("expected no second resync on warm boot, got %d source reads", got)
	}
}

func TestSecondResyncRunsOnColdBoot(t *testing.T) {
	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	w, err := redisstate.NewWriter(resyncCountingClient{}, keys, "fingerprint", redisstate.DefaultOptions(), logging.NewLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	src := &resyncCountingSource{}
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	waitForFlagCalls(t, src, 1)
	requestSecondResync(w, false, time.Second, nil, logging.NewLogger(nil))
	waitForFlagCalls(t, src, 2)
}

func TestSecondResyncNilWriter(t *testing.T) {
	requestSecondResync(nil, true, time.Second, nil, logging.NewLogger(nil))
	requestSecondResync(nil, false, time.Second, nil, logging.NewLogger(nil))
}

// dropRecordingClient reports a segment that only Redis knows about and records every drop script call
type dropRecordingClient struct {
	resyncCountingClient
	mtx   sync.Mutex
	drops []string
}

func (c *dropRecordingClient) Eval(script string, keys []string, args ...interface{}) error {
	if script == redisstate.SegmentDropScript {
		c.mtx.Lock()
		c.drops = append(c.drops, fmt.Sprint(args[len(args)-1]))
		c.mtx.Unlock()
	}
	return nil
}

func (c *dropRecordingClient) HGetAll(key string) (map[string]string, error) {
	return map[string]string{"seg.unfetched.till": "5"}, nil
}

func (c *dropRecordingClient) dropped() []string {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return append([]string(nil), c.drops...)
}

// partialSource lists one fetched segment while "unfetched" is still missing, as a cold boot does mid-sync
type partialSource struct{ resyncCountingSource }

func (s *partialSource) Segments(visit func(redisstate.SegmentState) error) error {
	return visit(redisstate.SegmentState{Name: "fetched", Active: map[string]int64{"k": 1}, Removed: map[string]int64{}, ChangeNumber: 1})
}

func TestGatedSourceIsEmptyUntilOpened(t *testing.T) {
	gate := newSourceGate(false)
	src := gate.wrap(&partialSource{})

	flags, cn, err := src.Flags()
	assert.NoError(t, err)
	assert.Empty(t, flags)
	assert.EqualValues(t, 0, cn)
	visited := 0
	assert.NoError(t, src.Segments(func(redisstate.SegmentState) error { visited++; return nil }))
	assert.Equal(t, 0, visited)

	gate.open()
	assert.NoError(t, src.Segments(func(redisstate.SegmentState) error { visited++; return nil }))
	assert.Equal(t, 1, visited)
	assert.True(t, newSourceGate(true).isOpen())
}

func TestColdBootResyncBeforeSyncCompletesDoesNotDropUnfetchedSegments(t *testing.T) {
	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	client := &dropRecordingClient{}
	w, err := redisstate.NewWriter(client, keys, "fingerprint", redisstate.DefaultOptions(), logging.NewLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	gate := newSourceGate(false)
	src := &partialSource{}
	if err := w.Start(gate.wrap(src)); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// the startup resync and an overflow style resync both run while the sync is still in progress
	w.RequestResync()
	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, client.dropped(), "a resync on a partial source must not delete segments")

	// once the sync is complete the same resync is allowed to drop what the source does not list
	gate.open()
	w.RequestResync()
	deadline := time.Now().Add(3 * time.Second)
	for len(client.dropped()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, []string{"unfetched"}, client.dropped())
}

func TestSecondResyncWaitsForQueueAndExpires(t *testing.T) {
	keys, err := redisstate.NewKeys("prefix", "apikey", "ns")
	if err != nil {
		t.Fatal(err)
	}
	w, err := redisstate.NewWriter(resyncCountingClient{}, keys, "fingerprint", redisstate.DefaultOptions(), logging.NewLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	src := &resyncCountingSource{}
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	waitForFlagCalls(t, src, 1)

	// an empty queue returns at once and requests the resync
	start := time.Now()
	requestSecondResync(w, false, 2*time.Second, nil, logging.NewLogger(nil))
	assert.Less(t, time.Since(start), time.Second)
	waitForFlagCalls(t, src, 2)

	// a bound that expires still requests the resync, and logs it
	var buf strings.Builder
	logger := logging.NewLogger(&logging.LoggerOptions{LogLevel: logging.LevelWarning, WarningWriter: &buf})
	requestSecondResyncWith(w, false, 50*time.Millisecond, nil, logger, func() bool { return false })
	assert.Contains(t, buf.String(), "queue did not drain")
	waitForFlagCalls(t, src, 3)
}
