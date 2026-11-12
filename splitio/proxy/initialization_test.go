package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	config "github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-split-commons/v10/synchronizer"
	"github.com/splitio/go-toolkit/v5/logging"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	commonconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
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

func TestMaskRedisStateSecretsMasksCopyOnly(t *testing.T) {
	live := &pconf.Main{}
	live.Storage.Redis.Connection.Username = "redis-user"
	live.Storage.Redis.Connection.Pass = "placeholder-pw"
	live.Storage.Redis.Connection.TLSClientKey = "client-key-pem-placeholder"
	live.Storage.Redis.Connection.Host = "redis.local"

	cfgForAdmin := *live
	maskRedisStateSecrets(&cfgForAdmin)

	conn := cfgForAdmin.Storage.Redis.Connection
	if conn.Username != maskedSecret || conn.Pass != maskedSecret || conn.TLSClientKey != maskedSecret {
		t.Fatalf("expected masked credentials in admin copy, got user=%q pass=%q key=%q", conn.Username, conn.Pass, conn.TLSClientKey)
	}
	if conn.Host != "redis.local" {
		t.Errorf("non-secret host should not be masked, got %q", conn.Host)
	}

	orig := live.Storage.Redis.Connection
	if orig.Username != "redis-user" || orig.Pass != "placeholder-pw" || orig.TLSClientKey != "client-key-pem-placeholder" {
		t.Errorf("live config was mutated: user=%q pass=%q key=%q", orig.Username, orig.Pass, orig.TLSClientKey)
	}

	empty := &pconf.Main{}
	maskRedisStateSecrets(empty)
	if empty.Storage.Redis.Connection.Pass != "" || empty.Storage.Redis.Connection.Username != "" {
		t.Error("empty credentials should stay empty so the admin view shows they are unset")
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

// startRequireRedis skips the test when no Redis answers on localhost:6379; with CI=true it fails instead.
func startRequireRedis(t *testing.T) {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DialTimeout: time.Second, MaxRetries: -1})
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := client.Ping(ctx).Err()
	if err == nil {
		return
	}
	if os.Getenv("CI") == "true" {
		t.Fatalf("redis is required on localhost:6379 in CI: %v", err)
	}
	t.Skipf("redis is not reachable on localhost:6379, skipping: %v", err)
}

// startTestLogger records every log line so tests can wait for one or inspect the boot decision.
type startTestLogger struct {
	logging.LoggerInterface
	mu   sync.Mutex
	msgs []string
}

func (l *startTestLogger) add(level string, msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, level+": "+fmt.Sprint(msg...))
}

func (l *startTestLogger) Error(msg ...interface{})   { l.add("error", msg...) }
func (l *startTestLogger) Warning(msg ...interface{}) { l.add("warning", msg...) }
func (l *startTestLogger) Info(msg ...interface{})    { l.add("info", msg...) }
func (l *startTestLogger) Debug(msg ...interface{})   { l.add("debug", msg...) }
func (l *startTestLogger) Verbose(msg ...interface{}) { l.add("verbose", msg...) }

func (l *startTestLogger) has(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func (l *startTestLogger) dump() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.msgs, "\n")
}

// startTestPrefix returns a Redis key prefix for one test; its keys are deleted at cleanup.
func startTestPrefix(t *testing.T) string {
	t.Helper()
	prefix := fmt.Sprintf("ftest-%d:", time.Now().UnixNano())
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() {
		ctx := context.Background()
		if keys, err := client.Keys(ctx, prefix+"*").Result(); err == nil && len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
		_ = client.Close()
	})
	return prefix
}

// startTestConfig builds a proxy config on free ports with Redis state under prefix and cloud URLs at cloudURL.
func startTestConfig(t *testing.T, cloudURL string, prefix string) *pconf.Main {
	t.Helper()
	cfg := &pconf.Main{}
	if err := commonconf.PopulateDefaults(cfg); err != nil {
		t.Fatalf("PopulateDefaults: %v", err)
	}
	cfg.Apikey = "startTestApikey-0123456789abcdef"
	cfg.FlagSpecVersion = "1.2"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freeTCPPort(t)
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = freeTCPPort(t)
	cfg.Sync.Advanced.StreamingEnabled = false
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.Namespace = "startns"
	cfg.Storage.Redis.Connection = commonconf.Redis{Host: "localhost", Port: 6379, Prefix: prefix}
	t.Setenv("SPLIT_PROXY_SDK_URL", cloudURL)
	t.Setenv("SPLIT_PROXY_EVENTS_URL", cloudURL)
	t.Setenv("SPLIT_PROXY_AUTH_SERVICE_URL", cloudURL)
	t.Setenv("SPLIT_PROXY_STREAMING_SERVICE_URL", cloudURL)
	t.Setenv("SPLIT_PROXY_TELEMETRY_SERVICE_URL", cloudURL)
	return cfg
}

func freeTCPPort(t *testing.T) int64 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return int64(port)
}

// closedCloudURL returns a URL that refuses connections, simulating Split cloud down.
func closedCloudURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

// startInBackground runs Start and reports its return value; tests wait on log lines since success blocks.
func startInBackground(logger logging.LoggerInterface, cfg *pconf.Main) <-chan error {
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()
	return done
}

// startChildEnv names the test whose body runs in this child process.
const startChildEnv = "SPLIT_PROXY_START_CHILD_TEST"

// isolatedStartTest runs a Start-level test body in a child test process and reports true inside that child.
// A successful Start blocks until shutdown and leaves goroutines running, so the body runs in its own process.
func isolatedStartTest(t *testing.T) bool {
	t.Helper()
	if os.Getenv(startChildEnv) == t.Name() {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), startChildEnv+"="+t.Name())
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: "+t.Name()) {
		t.Fatalf("child run of %s failed: %v\n%s", t.Name(), err, out)
	}
	return false
}

// waitForStartLog fails if Start returns before want is logged, or if want is not logged within timeout.
func waitForStartLog(t *testing.T, done <-chan error, logger *startTestLogger, want string, timeout time.Duration) {
	t.Helper()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(timeout)
	for {
		select {
		case err := <-done:
			t.Fatalf("Start returned before %q was logged: %v\nlogs:\n%s", want, err, logger.dump())
		case <-tick.C:
			if logger.has(want) {
				return
			}
		case <-deadline:
			t.Fatalf("%q was not logged within %s\nlogs:\n%s", want, timeout, logger.dump())
		}
	}
}

func exitCodeOf(err error) (int, bool) {
	var initErr *common.InitializationError
	if errors.As(err, &initErr) {
		return initErr.ExitCode(), true
	}
	return 0, false
}

// assertStartExit fails unless err is an InitializationError with the wanted exit code.
func assertStartExit(t *testing.T, err error, want int, logger *startTestLogger) {
	t.Helper()
	code, ok := exitCodeOf(err)
	if !ok {
		t.Fatalf("Start must return an InitializationError, got %v\nlogs:\n%s", err, logger.dump())
	}
	if code != want {
		t.Fatalf("exit code = %d, want %d: %v\nlogs:\n%s", code, want, err, logger.dump())
	}
}

// startTestFlag is a minimal active flag for the redis state writer.
func startTestFlag(name string, cn int64) dtos.SplitDTO {
	return dtos.SplitDTO{Name: name, Status: "ACTIVE", ChangeNumber: cn, TrafficTypeName: "user", DefaultTreatment: "off", Conditions: []dtos.ConditionDTO{}}
}

// startSeedRedisFlags writes one flag through the redis state writer so the boot read finds a valid state,
// and waits for it to apply.
func startSeedRedisFlags(t *testing.T, cfg *pconf.Main) redisstate.Keys {
	t.Helper()
	logger := logging.NewLogger(nil)
	wclient, err := commonsredis.NewRedisClient(&config.RedisConfig{Host: "localhost", Port: 6379, Database: 0, Network: "tcp", Prefix: cfg.Storage.Redis.Connection.Prefix}, logger)
	if err != nil {
		t.Fatalf("writer redis client: %v", err)
	}
	keys, err := redisstate.NewKeys(wclient.Prefix(), cfg.Apikey, cfg.Storage.Redis.Namespace)
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	fingerprint := util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
	w, err := redisstate.NewWriter(wclient, keys, fingerprint, redisstate.DefaultOptions(), logger)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Start(nil); err != nil {
		t.Fatalf("writer start: %v", err)
	}
	w.PutFlags([]dtos.SplitDTO{startTestFlag("startFlag", 3)}, nil, 3)
	deadline := time.Now().Add(30 * time.Second)
	for w.Status().PendingChanges > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("writer did not drain its queue")
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	return keys
}

// startAddFlagDataWithoutIndex adds a flags data entry the flags index does not hold, so the reader sees a mismatch.
func startAddFlagDataWithoutIndex(t *testing.T, keys redisstate.Keys) {
	t.Helper()
	raw := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer func() { _ = raw.Close() }()
	if err := raw.HSet(context.Background(), keys.FlagData(), "ghost-flag", `{"name":"ghost-flag","changeNumber":3}`).Err(); err != nil {
		t.Fatalf("add flag data without index: %v", err)
	}
}

// writeBadSnapshotFile writes bytes that are not a snapshot and returns the path.
func writeBadSnapshotFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bad.snap")
	if err := os.WriteFile(path, []byte("not a snapshot"), 0o600); err != nil {
		t.Fatalf("write bad snapshot: %v", err)
	}
	return path
}

// Redis empty and cloud down with no snapshot file: Start returns an InitError with ExitTaskInitialization.
func TestStartEmptyRedisCloudDownNoSnapshotFails(t *testing.T) {
	startRequireRedis(t)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	logger := &startTestLogger{}
	err := start(logger, cfg)
	assertStartExit(t, err, common.ExitTaskInitialization, logger)
	if !logger.has("no redis state found") {
		t.Errorf("expected the absent redis state to be logged\nlogs:\n%s", logger.dump())
	}
}

// A valid redis state wins over a snapshot file that cannot be parsed. Start must not fail on the file.
// The success path blocks, so it runs in a child process.
func TestStartValidRedisBeatsBadSnapshotFile(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	startSeedRedisFlags(t, cfg)
	cfg.Initialization.Snapshot = writeBadSnapshotFile(t)

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)
	waitForStartLog(t, done, logger, "Failed to perform initial sync", 60*time.Second)

	if !logger.has("starting from redis state") {
		t.Errorf("expected redis to be the boot source\nlogs:\n%s", logger.dump())
	}
	if logger.has("error parsing snapshot file") {
		t.Errorf("the snapshot file must be ignored when redis is valid\nlogs:\n%s", logger.dump())
	}
}

// With no usable redis state the snapshot file is read; an unparsable file fails boot with
// ExitInvalidConfiguration.
func TestStartSnapshotFileReadWhenRedisEmpty(t *testing.T) {
	startRequireRedis(t)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	cfg.Initialization.Snapshot = writeBadSnapshotFile(t)
	logger := &startTestLogger{}
	err := start(logger, cfg)
	assertStartExit(t, err, common.ExitInvalidConfiguration, logger)
	if !strings.Contains(err.Error(), "error parsing snapshot file") {
		t.Errorf("expected the snapshot parse error, got %v\nlogs:\n%s", err, logger.dump())
	}
}

// A flags index/data mismatch is Corrupt; nothing is restored and Start fails with ExitTaskInitialization.
func TestStartCorruptFlagIndexMismatchFails(t *testing.T) {
	startRequireRedis(t)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	keys := startSeedRedisFlags(t, cfg)
	startAddFlagDataWithoutIndex(t, keys)
	checkEmpty := assertLiveStoragesEmpty(t)

	logger := &startTestLogger{}
	err := start(logger, cfg)
	assertStartExit(t, err, common.ExitTaskInitialization, logger)
	checkEmpty()
	if !logger.has("redis state is corrupt") {
		t.Errorf("expected the corrupt redis state to be logged\nlogs:\n%s", logger.dump())
	}
}

// startRedisFlagCursor returns the flag cursor field committed in Redis under keys.
func startRedisFlagCursor(t *testing.T, keys redisstate.Keys) string {
	t.Helper()
	raw := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer func() { _ = raw.Close() }()
	got, err := raw.HGet(context.Background(), keys.Meta(), keys.FlagCursorField()).Result()
	if err != nil {
		t.Fatalf("read flag cursor from redis: %v", err)
	}
	return got
}

// startRedisErrorLogs returns the error-level log lines that mention redis.
func startRedisErrorLogs(logger *startTestLogger) []string {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	var out []string
	for _, m := range logger.msgs {
		if strings.HasPrefix(m, "error: ") && strings.Contains(strings.ToLower(m), "redis") {
			out = append(out, m)
		}
	}
	return out
}

// A valid redis state with the cloud down serves from redis, keeps retrying the sync, and Start does not return.
func TestStartValidRedisCloudDownServesFromRedis(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	startSeedRedisFlags(t, cfg)

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)
	waitForStartLog(t, done, logger, "Failed to perform initial sync", 60*time.Second)

	if !logger.has("starting from redis state") {
		t.Errorf("expected redis to be the boot source\nlogs:\n%s", logger.dump())
	}
	select {
	case err := <-done:
		t.Fatalf("Start returned while the cloud is down: %v\nlogs:\n%s", err, logger.dump())
	case <-time.After(500 * time.Millisecond):
	}
}

// Valid redis state with cloud up; the first splitChanges request asks for the committed flag cursor.
func TestStartValidRedisCloudUpSyncsFromCommittedCursor(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	var mu sync.Mutex
	var sinces []string
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/splitChanges") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		since := r.URL.Query().Get("since")
		mu.Lock()
		sinces = append(sinces, since)
		mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"splits":[],"since":%s,"till":%s}`, since, since)
	}))
	defer cloud.Close()

	cfg := startTestConfig(t, cloud.URL, startTestPrefix(t))
	keys := startSeedRedisFlags(t, cfg)
	committed := startRedisFlagCursor(t, keys)
	if committed != "3" {
		t.Fatalf("seeded flag cursor in redis: want 3, got %q", committed)
	}

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)
	waitForStartLog(t, done, logger, "Synchronizer tasks started", 60*time.Second)

	mu.Lock()
	got := append([]string(nil), sinces...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatalf("the cloud received no splitChanges request\nlogs:\n%s", logger.dump())
	}
	if got[0] != committed {
		t.Errorf("first splitChanges since = %q, want the committed flag cursor %q\nlogs:\n%s", got[0], committed, logger.dump())
	}
}

// A proxy TLS failure after a valid redis boot must return ExitTLSError without leaving the admin server serving.
func TestStartServerTLSErrorDoesNotStartAdmin(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/splitChanges") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		since := r.URL.Query().Get("since")
		_, _ = fmt.Fprintf(w, `{"splits":[],"since":%s,"till":%s}`, since, since)
	}))
	defer cloud.Close()

	cfg := startTestConfig(t, cloud.URL, startTestPrefix(t))
	startSeedRedisFlags(t, cfg)
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertChainFN = filepath.Join(t.TempDir(), "missing-cert.pem")
	cfg.Server.TLS.PrivateKeyFN = filepath.Join(t.TempDir(), "missing-key.pem")

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)
	var err error
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("Start did not return with a broken proxy TLS config\nlogs:\n%s", logger.dump())
	}
	assertStartExit(t, err, common.ExitTLSError, logger)

	addr := net.JoinHostPort(cfg.Admin.Host, strconv.FormatInt(cfg.Admin.Port, 10))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			t.Fatalf("admin server answered on %s although Start returned ExitTLSError\nlogs:\n%s", addr, logger.dump())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// The afterRestore hook sees the restored flag cursor, and the writer startup resync does not reset it.
func TestStartAfterRestoreSeesCursorAndWriterKeepsIt(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	keys := startSeedRedisFlags(t, cfg)

	cursorAtHook := int64(-1)
	var hookErr error
	afterRestoreHook = func(splits *storage.ProxySplitStorageImpl) {
		cursorAtHook, hookErr = splits.ChangeNumber()
	}
	t.Cleanup(func() { afterRestoreHook = nil })
	logger := &startTestLogger{}
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()
	waitForStartLog(t, done, logger, "Failed to perform initial sync", 60*time.Second)

	if hookErr != nil {
		t.Fatalf("read local flag cursor in afterRestore: %v", hookErr)
	}
	if cursorAtHook != 3 {
		t.Errorf("local flag cursor at afterRestore: want 3 (restored value), got %d\nlogs:\n%s", cursorAtHook, logger.dump())
	}
	if got := startRedisFlagCursor(t, keys); got != "3" {
		t.Errorf("redis flag cursor after writer start: want 3, got %q\nlogs:\n%s", got, logger.dump())
	}
	if errs := startRedisErrorLogs(logger); len(errs) > 0 {
		t.Errorf("writer startup logged redis errors: %v\nlogs:\n%s", errs, logger.dump())
	}
}

// A boot from redis does not log "snapshot could not be loaded", even with a snapshot path configured.
func TestStartRedisBootLogsNoSnapshotLoadWarning(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	startSeedRedisFlags(t, cfg)
	cfg.Initialization.Snapshot = filepath.Join(t.TempDir(), "missing.snap")

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)
	waitForStartLog(t, done, logger, "Failed to perform initial sync", 60*time.Second)

	if !logger.has("starting from redis state") {
		t.Errorf("expected redis to be the boot source\nlogs:\n%s", logger.dump())
	}
	if logger.has("snapshot could not be loaded") {
		t.Errorf("a redis boot must not log snapshot load warnings\nlogs:\n%s", logger.dump())
	}
}

// startBootMatrixCell runs one boot-matrix cell at Start level; success paths run in a child process via isolatedStartTest.
func startBootMatrixCell(t *testing.T, state, cloud string) {
	t.Helper()
	wantErr := 0
	switch {
	case state == "unreachable":
		wantErr = common.ExitRedisInitializationFailed
	case cloud == "down" && (state == "absent" || state == "corrupt" || state == "foreign"):
		wantErr = common.ExitTaskInitialization
	}
	if state != "unreachable" {
		startRequireRedis(t)
	}
	if wantErr == 0 && !isolatedStartTest(t) {
		return
	}

	const staleCloudCN = "5"
	var mu sync.Mutex
	var sinces []string
	cloudURL := closedCloudURL(t)
	if cloud == "up" {
		cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if !strings.HasSuffix(r.URL.Path, "/splitChanges") {
				_, _ = w.Write([]byte(`{}`))
				return
			}
			since := r.URL.Query().Get("since")
			mu.Lock()
			sinces = append(sinces, since)
			mu.Unlock()
			if state == "stale" && since == "3" {
				_, _ = fmt.Fprintf(w, `{"splits":[{"name":"startFlag","status":"ACTIVE","changeNumber":%s,"trafficTypeName":"user","defaultTreatment":"off","conditions":[]}],"since":3,"till":%s}`, staleCloudCN, staleCloudCN)
				return
			}
			_, _ = fmt.Fprintf(w, `{"splits":[],"since":%s,"till":%s}`, since, since)
		}))
		defer cloudSrv.Close()
		cloudURL = cloudSrv.URL
	}

	prefix := startTestPrefix(t)
	cfg := startTestConfig(t, cloudURL, prefix)
	switch state {
	case "corrupt":
		startAddFlagDataWithoutIndex(t, startSeedRedisFlags(t, cfg))
	case "foreign":
		foreign := startTestConfig(t, cloudURL, prefix)
		foreign.FlagSpecVersion = "1.1"
		startSeedRedisFlags(t, foreign)
	case "stale":
		startSeedRedisFlags(t, cfg)
	case "unreachable":
		cfg.Storage.Redis.Connection.Port = 1
	}

	logger := &startTestLogger{}
	if wantErr != 0 {
		err := start(logger, cfg)
		assertStartExit(t, err, wantErr, logger)
		return
	}

	waitFor := "Synchronizer tasks started"
	if cloud == "down" {
		waitFor = "Failed to perform initial sync"
	}
	done := startInBackground(logger, cfg)
	waitForStartLog(t, done, logger, waitFor, 60*time.Second)

	if state == "absent" && !logger.has("no redis state found") {
		t.Errorf("expected the absent redis state to be logged\nlogs:\n%s", logger.dump())
	}
	if state == "corrupt" && !logger.has("redis state is corrupt") {
		t.Errorf("expected the corrupt redis state to be logged\nlogs:\n%s", logger.dump())
	}
	fromRedis := state == "stale"
	if logger.has("starting from redis state") != fromRedis {
		t.Errorf("boot source from redis = %v, want %v\nlogs:\n%s", !fromRedis, fromRedis, logger.dump())
	}

	if cloud == "down" {
		select {
		case err := <-done:
			t.Fatalf("Start returned while the cloud is down: %v\nlogs:\n%s", err, logger.dump())
		case <-time.After(500 * time.Millisecond):
		}
		return
	}
	wantSince := "-1"
	if fromRedis {
		wantSince = "3"
	}
	mu.Lock()
	got := append([]string(nil), sinces...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatalf("the cloud received no splitChanges request\nlogs:\n%s", logger.dump())
	}
	if got[0] != wantSince {
		t.Errorf("first splitChanges since = %q, want %q\nlogs:\n%s", got[0], wantSince, logger.dump())
	}
}

// TestStartBootMatrix covers the boot-matrix cells not covered by the tests above. Covered by name elsewhere:
// redis_absent/cloud_down (TestStartEmptyRedisCloudDownNoSnapshotFails),
// redis_valid/cloud_down (TestStartValidRedisCloudDownServesFromRedis),
// redis_valid/cloud_up (TestStartValidRedisCloudUpSyncsFromCommittedCursor),
// redis_corrupt/cloud_down (TestStartCorruptFlagIndexMismatchFails).
func TestStartBootMatrix(t *testing.T) {
	states := []struct {
		name   string
		clouds []string
	}{
		{"absent", []string{"up"}},
		{"corrupt", []string{"up"}},
		{"foreign", []string{"up", "down"}},
		{"stale", []string{"up", "down"}},
		{"unreachable", []string{"up", "down"}},
	}
	for _, s := range states {
		t.Run("redis_"+s.name, func(t *testing.T) {
			for _, cloud := range s.clouds {
				t.Run("cloud_"+cloud, func(t *testing.T) {
					startBootMatrixCell(t, s.name, cloud)
				})
			}
		})
	}
}

// The admin health endpoints answer while the redis boot read is still in progress, and the port is handed
// to the full admin server afterwards.
func TestStartAdminAnswersDuringSlowBootRead(t *testing.T) {
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	entered := make(chan struct{})
	release := make(chan struct{})
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) {
		close(entered)
		<-release
		return nil, common.NewInitError(errors.New("slow boot released"), common.ExitRedisInitializationFailed)
	}
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatalf("boot read never started\nlogs:\n%s", logger.dump())
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.Admin.Port)
	for _, path := range []string{"/health/application", "/health/dependencies"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s during the boot read: %v", path, err)
		}
		var body struct {
			Healthy *bool                  `json:"healthy"`
			Items   *[]interface{}         `json:"items"`
			Redis   map[string]interface{} `json:"redis"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s during the boot read = %d, want 200", path, resp.StatusCode)
		}
		if decodeErr != nil || body.Healthy == nil || *body.Healthy || body.Items == nil {
			t.Errorf("GET %s body must keep the health shape with healthy=false: %+v (%v)", path, body, decodeErr)
		}
		if body.Redis["bootPhase"] != bootPhaseReading || body.Redis["enabled"] != true {
			t.Errorf("GET %s redis metadata = %v, want enabled and bootPhase=%s", path, body.Redis, bootPhaseReading)
		}
	}
	resp, err := http.Get(base + "/admin/dashboard")
	if err != nil {
		t.Fatalf("GET /admin/dashboard during the boot read: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("dashboard during the boot read = %d, want 503", resp.StatusCode)
	}

	close(release)
	select {
	case err := <-done:
		assertStartExit(t, err, common.ExitRedisInitializationFailed, logger)
	case <-time.After(10 * time.Second):
		t.Fatalf("start did not return after the boot read failed\nlogs:\n%s", logger.dump())
	}
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port))
	if err != nil {
		t.Fatalf("the boot admin server must release the port when start fails: %v", err)
	}
	_ = l.Close()
}

// A port that cannot be bound fails the boot with ExitAdminError before the redis read.
func TestStartBootAdminPortInUseFailsWithAdminError(t *testing.T) {
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port))
	if err != nil {
		t.Fatalf("occupy admin port: %v", err)
	}
	defer func() { _ = l.Close() }()
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) {
		t.Error("the redis read must not start when the admin port is taken")
		return nil, errors.New("unexpected")
	}
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	assertStartExit(t, start(logger, cfg), common.ExitAdminError, logger)
}

// assertLiveStoragesEmpty installs afterRestoreHook and returns a check that it ran once and saw empty storages:
// a non-redis outcome must not leave partly restored boot data in the storages the proxy serves from.
func assertLiveStoragesEmpty(t *testing.T) func() {
	t.Helper()
	calls := 0
	var flags int
	var cn int64
	afterRestoreHook = func(splits *storage.ProxySplitStorageImpl) {
		calls++
		flags = len(splits.All())
		cn, _ = splits.ChangeNumber()
	}
	t.Cleanup(func() { afterRestoreHook = nil })
	return func() {
		t.Helper()
		if calls != 1 {
			t.Fatalf("afterRestoreHook calls = %d, want 1", calls)
		}
		if flags != 0 || cn != -1 {
			t.Errorf("live storages not empty after a non-redis outcome: flags=%d changeNumber=%d", flags, cn)
		}
	}
}

// A redis read that is not used discards its throwaway boot database.
func TestStartNonRedisOutcomeDiscardsBootDB(t *testing.T) {
	startRequireRedis(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	keys := startSeedRedisFlags(t, cfg)
	startAddFlagDataWithoutIndex(t, keys)

	logger := &startTestLogger{}
	err := start(logger, cfg)
	assertStartExit(t, err, common.ExitTaskInitialization, logger)

	left, globErr := filepath.Glob(filepath.Join(tmp, "splitio_*.db"))
	if globErr != nil {
		t.Fatalf("glob temp dir: %v", globErr)
	}
	// The cold-start db built after the discard is a fresh file; a failed boot discards it too (deferred closure).
	if len(left) != 0 {
		t.Errorf("throwaway boot database left behind: %v\nlogs:\n%s", left, logger.dump())
	}
}

// Absent and foreign redis state also throw the boot database away (the corrupt case is covered above).
func TestStartAbsentAndForeignOutcomesDiscardBootDB(t *testing.T) {
	startRequireRedis(t)
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, cfg *pconf.Main)
	}{
		{"absent", func(*testing.T, *pconf.Main) {}},
		{"foreign", func(t *testing.T, cfg *pconf.Main) {
			client := bootPopulatedRedis(t, cfg, 0)
			client.hashes[bootTestKeys(t, cfg).Meta()][redisstate.FingerprintField] = "other-environment"
			orig := newBootReadClient
			newBootReadClient = func(*pconf.Main) (bootReadClient, error) { return client, nil }
			t.Cleanup(func() { newBootReadClient = orig })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
			tc.setup(t, cfg)
			checkEmpty := assertLiveStoragesEmpty(t)

			logger := &startTestLogger{}
			assertStartExit(t, start(logger, cfg), common.ExitTaskInitialization, logger)
			checkEmpty()

			left, globErr := filepath.Glob(filepath.Join(tmp, "splitio_*.db"))
			if globErr != nil {
				t.Fatalf("glob temp dir: %v", globErr)
			}
			if len(left) != 0 {
				t.Errorf("databases left behind: %v\nlogs:\n%s", left, logger.dump())
			}
		})
	}
}

// A redis boot that fails after the restore discards its throwaway boot database.
func TestStartRedisBootFailureAfterRestoreDiscardsBootDB(t *testing.T) {
	startRequireRedis(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	client := bootPopulatedRedis(t, cfg, 0)
	delete(client.hashes[bootTestKeys(t, cfg).Meta()], bootTestKeys(t, cfg).SegmentCursorField("s2"))
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) { return client, nil }
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()

	select {
	case err := <-done:
		assertStartExit(t, err, common.ExitTaskInitialization, logger)
	case <-time.After(60 * time.Second):
		t.Fatalf("start did not return after a failed cloud sync\nlogs:\n%s", logger.dump())
	}

	left, globErr := filepath.Glob(filepath.Join(tmp, "splitio_*.db"))
	if globErr != nil {
		t.Fatalf("glob temp dir: %v", globErr)
	}
	if len(left) != 0 {
		t.Errorf("throwaway boot database left behind after a failed redis boot: %v\nlogs:\n%s", left, logger.dump())
	}
}

// A segment name over bolt's 32768-byte key limit makes the redis restore sink fail while writing segments.
// start must return the redis init exit (not a cold start), discard the boot DB and close the read client.
func TestStartRedisRestoreSinkFailureIsRedisInitError(t *testing.T) {
	startRequireRedis(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	client := bootPopulatedRedis(t, cfg, 0)
	keys := bootTestKeys(t, cfg)
	longName := strings.Repeat("g", 40000)
	client.hashes[keys.RuleBasedSegmentData()]["r1"] = bootRbsJSON(t, "r1", 20, longName)
	client.hashes[keys.Meta()][keys.SegmentCursorField(longName)] = "30"
	client.setScores(keys.SegmentActive(longName), map[string]int64{"k1": 30})
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) { return client, nil }
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()

	select {
	case err := <-done:
		assertStartExit(t, err, common.ExitRedisInitializationFailed, logger)
	case <-time.After(60 * time.Second):
		t.Fatalf("start did not return after a failed redis restore\nlogs:\n%s", logger.dump())
	}

	if client.closes != 1 {
		t.Errorf("read client closed %d times, want 1", client.closes)
	}
	left, globErr := filepath.Glob(filepath.Join(tmp, "splitio_*.db"))
	if globErr != nil {
		t.Fatalf("glob temp dir: %v", globErr)
	}
	if len(left) != 0 {
		t.Errorf("boot database left behind after a redis restore failure: %v\nlogs:\n%s", left, logger.dump())
	}
}

// A redis boot that skipped a segment is incomplete, so a failed first cloud sync is fatal instead of
// retried in the background with partial data.
func TestStartSkippedSegmentMakesFailedCloudSyncFatal(t *testing.T) {
	startRequireRedis(t)
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	client := bootPopulatedRedis(t, cfg, 0)
	delete(client.hashes[bootTestKeys(t, cfg).Meta()], bootTestKeys(t, cfg).SegmentCursorField("s2"))
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) { return client, nil }
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	done := make(chan error, 1)
	go func() { done <- start(logger, cfg) }()

	select {
	case err := <-done:
		assertStartExit(t, err, common.ExitTaskInitialization, logger)
		if !logger.has("were not restored from redis") {
			t.Errorf("expected the skipped segment warning\nlogs:\n%s", logger.dump())
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("start kept running after a failed cloud sync with a skipped segment\nlogs:\n%s", logger.dump())
	}
}

// Redis Cluster is not supported for the redis state in this version: start rejects it before any boot work.
func TestStartRejectsRedisClusterForRedisState(t *testing.T) {
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.Connection.ClusterMode = true
	cfg.Storage.Redis.Connection.ClusterNodes = "127.0.0.1:7000,127.0.0.1:7001"

	logger := &startTestLogger{}
	assertStartExit(t, start(logger, cfg), common.ExitInvalidConfiguration, logger)
}

func TestStartInvalidRedisSettingIsReportedBeforeAdminBind(t *testing.T) {
	cfg := startTestConfig(t, closedCloudURL(t), startTestPrefix(t))
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port))
	if err != nil {
		t.Fatalf("occupy admin port: %v", err)
	}
	defer func() { _ = l.Close() }()
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.ChunkSize = 0
	orig := newBootReadClient
	newBootReadClient = func(*pconf.Main) (bootReadClient, error) {
		t.Error("the redis read must not start with an invalid redis setting")
		return nil, errors.New("unexpected")
	}
	t.Cleanup(func() { newBootReadClient = orig })

	logger := &startTestLogger{}
	assertStartExit(t, start(logger, cfg), common.ExitInvalidConfiguration, logger)
}

func TestRedisClusterRequested(t *testing.T) {
	cases := []struct {
		name     string
		mode     bool
		sentinel bool
		nodes    string
		want     bool
	}{
		{name: "cluster mode", mode: true, nodes: "127.0.0.1:7000", want: true},
		{name: "cluster mode without nodes", mode: true, want: true},
		{name: "stale nodes with cluster mode off", nodes: "127.0.0.1:7000"},
		{name: "sentinel wins over cluster mode", mode: true, sentinel: true, nodes: "127.0.0.1:7000"},
		{name: "standalone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &pconf.Main{}
			cfg.Storage.Redis.Connection.ClusterMode = tc.mode
			cfg.Storage.Redis.Connection.SentinelReplication = tc.sentinel
			cfg.Storage.Redis.Connection.ClusterNodes = tc.nodes
			if got := redisClusterRequested(cfg); got != tc.want {
				t.Errorf("redisClusterRequested = %v, want %v", got, tc.want)
			}
		})
	}
}

// start() marks the boot phase as initial-sync before the first cloud sync, so the admin reports it during that sync.
func TestStartBootPhaseIsInitialSyncWhileFirstCloudSyncInFlight(t *testing.T) {
	startRequireRedis(t)
	if !isolatedStartTest(t) {
		return
	}
	inFlight := make(chan struct{})
	var inFlightOnce sync.Once
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/splitChanges") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		inFlightOnce.Do(func() { close(inFlight) })
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		since := r.URL.Query().Get("since")
		_, _ = fmt.Fprintf(w, `{"splits":[],"since":%s,"till":%s}`, since, since)
	}))
	defer cloud.Close()
	defer releaseOnce()

	cfg := startTestConfig(t, cloud.URL, startTestPrefix(t))
	startSeedRedisFlags(t, cfg)

	logger := &startTestLogger{}
	done := startInBackground(logger, cfg)

	select {
	case <-inFlight:
	case <-time.After(60 * time.Second):
		t.Fatalf("the first splitChanges request never reached the cloud\nlogs:\n%s", logger.dump())
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.Admin.Port)
	deadline := time.Now().Add(20 * time.Second)
	phase := ""
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health/application")
		if err == nil {
			var body struct {
				Redis map[string]interface{} `json:"redis"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&body)
			_ = resp.Body.Close()
			if decodeErr == nil {
				phase, _ = body.Redis["bootPhase"].(string)
				if phase == bootPhaseInitialSync {
					break
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if phase != bootPhaseInitialSync {
		t.Fatalf("boot phase during the first cloud sync = %q, want %q\nlogs:\n%s", phase, bootPhaseInitialSync, logger.dump())
	}

	releaseOnce()
	waitForStartLog(t, done, logger, "Synchronizer tasks started", 60*time.Second)
}
