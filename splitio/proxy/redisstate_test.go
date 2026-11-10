package proxy

import (
	"errors"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commonconf "github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	commonredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/stretchr/testify/assert"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
)

func redisStateTestConfig(t *testing.T) *pconf.Main {
	t.Helper()
	cfg := &pconf.Main{}
	cfg.Apikey = "redis-state-test-apikey"
	cfg.FlagSpecVersion = "1.2"
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.RetentionHours = 168
	cfg.Storage.Redis.WriteMaxAttempts = 3
	cfg.Storage.Redis.WriteBackoffMaxMs = 1000
	cfg.Storage.Redis.MaxPendingChanges = 1000
	cfg.Storage.Redis.ChunkSize = 1000
	cfg.Storage.Redis.PurgeBatch = 500
	cfg.Storage.Redis.StopTimeoutMs = 10000
	cfg.Storage.Redis.Connection.Host = "localhost"
	cfg.Storage.Redis.Connection.Port = 6379
	cfg.Storage.Redis.Connection.Prefix = "t-" + t.Name()
	cfg.Storage.Redis.Connection.DialTimeout = 5
	cfg.Storage.Redis.Connection.ReadTimeout = 5
	cfg.Storage.Redis.Connection.WriteTimeout = 5
	return cfg
}

func assertInitError(t *testing.T, err error, exitCode int) {
	t.Helper()
	var initErr *common.InitializationError
	if !assert.ErrorAs(t, err, &initErr) {
		return
	}
	assert.Equal(t, exitCode, initErr.ExitCode())
}

func unprefixedKeys(prefix string, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, strings.TrimPrefix(k, prefix+"."))
	}
	return out
}

func waitForRedisStateFlush(t *testing.T, writer *redisstate.Writer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for writer.Status().PendingChanges > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("redis state queue not flushed: %+v", writer.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSetupRedisStateDisabled(t *testing.T) {
	cfg := redisStateTestConfig(t)
	cfg.Storage.Redis.Enabled = false

	writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
	assert.NoError(t, err)
	assert.True(t, writer == nil)
}

func TestSetupRedisStateInvalidConfig(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *pconf.Main)
	}{
		{
			name: "tls ca file does not exist",
			setup: func(cfg *pconf.Main) {
				cfg.Storage.Redis.Connection.TLS = true
				cfg.Storage.Redis.Connection.TLSCACertificates = []string{filepath.Join(t.TempDir(), "missing-ca.pem")}
			},
		},
		{
			name: "tls client key without certificate",
			setup: func(cfg *pconf.Main) {
				cfg.Storage.Redis.Connection.TLS = true
				cfg.Storage.Redis.Connection.TLSClientKey = "client-key.pem"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			tc.setup(cfg)

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

func TestSetupRedisStateRejectsNonPositiveSettings(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *pconf.Main)
	}{
		{name: "chunkSize zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.ChunkSize = 0 }},
		{name: "chunkSize negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.ChunkSize = -1 }},
		{name: "chunkSize above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.ChunkSize = 10001 }},
		{name: "purgeBatch zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.PurgeBatch = 0 }},
		{name: "purgeBatch negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.PurgeBatch = -1 }},
		{name: "purgeBatch above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.PurgeBatch = 5001 }},
		{name: "stopTimeoutMs zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.StopTimeoutMs = 0 }},
		{name: "stopTimeoutMs negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.StopTimeoutMs = -1 }},
		{name: "stopTimeoutMs above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.StopTimeoutMs = 60001 }},
		{name: "maxPendingChanges zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.MaxPendingChanges = 0 }},
		{name: "maxPendingChanges negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.MaxPendingChanges = -1 }},
		{name: "retentionHours zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.RetentionHours = 0 }},
		{name: "retentionHours negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.RetentionHours = -1 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			tc.setup(cfg)

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

func TestSetupRedisStateRetentionHoursOverflow(t *testing.T) {
	tests := []struct {
		name  string
		hours int64
	}{
		{name: "hour to ms conversion overflows int64", hours: math.MaxInt64/3600000 + 1},
		{name: "hours to nanosecond duration overflows", hours: math.MaxInt64/int64(time.Hour) + 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			cfg.Storage.Redis.RetentionHours = tc.hours

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

func TestSetupRedisStateRejectsNonPositiveRetrySettings(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *pconf.Main)
	}{
		{name: "writeMaxAttempts zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteMaxAttempts = 0 }},
		{name: "writeMaxAttempts negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteMaxAttempts = -1 }},
		{name: "writeBackoffMaxMs zero", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteBackoffMaxMs = 0 }},
		{name: "writeBackoffMaxMs negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteBackoffMaxMs = -1 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			tc.setup(cfg)

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

func TestSetupRedisStateRejectsOutOfRangeRetryAndConnectionSettings(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *pconf.Main)
	}{
		{name: "writeMaxAttempts above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteMaxAttempts = 101 }},
		{name: "writeBackoffMaxMs above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.WriteBackoffMaxMs = 3600001 }},
		{name: "maxPendingChanges above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.MaxPendingChanges = 10000001 }},
		{name: "dialTimeout negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.DialTimeout = -1 }},
		{name: "readTimeout negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.ReadTimeout = -1 }},
		{name: "writeTimeout negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.WriteTimeout = -1 }},
		{name: "dialTimeout above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.DialTimeout = 301 }},
		{name: "readTimeout above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.ReadTimeout = 301 }},
		{name: "writeTimeout above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.WriteTimeout = 301 }},
		{name: "maxRetries negative", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.MaxRetries = -1 }},
		{name: "maxRetries above upper bound", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.Connection.MaxRetries = 11 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			tc.setup(cfg)

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

func TestRedisStateLimitsFollowWriterLimits(t *testing.T) {
	tests := []struct {
		name  string
		setup func(cfg *pconf.Main)
	}{
		{name: "chunkSize above writer limit", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.ChunkSize = redisstate.MaxChunkSize + 1 }},
		{name: "purgeBatch above writer limit", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.PurgeBatch = redisstate.MaxPurgeBatch + 1 }},
		{name: "stopTimeoutMs above writer limit", setup: func(cfg *pconf.Main) { cfg.Storage.Redis.StopTimeoutMs = redisstate.MaxStopTimeoutMs + 1 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redisStateTestConfig(t)
			tc.setup(cfg)

			writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
			assert.True(t, writer == nil)
			assertInitError(t, err, common.ExitInvalidConfiguration)
		})
	}
}

// fakeRedisProbeFailureServer answers PING so NewRedisClient succeeds and answers every other command with a Redis
// error reply, which is what the Writer.Start probe receives from a server that can not run the scripts.
var redisArrayHeader = regexp.MustCompile(`(?m)^\*\d+\r?\n`)

func fakeRedisProbeFailureServer(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					data := buf[:n]
					// One reply per command, so pipelined commands stay in step with the client.
					locs := redisArrayHeader.FindAllIndex(data, -1)
					if len(locs) == 0 {
						locs = [][]int{{0, 0}}
					}
					var replies strings.Builder
					for i, loc := range locs {
						end := len(data)
						if i+1 < len(locs) {
							end = locs[i+1][0]
						}
						if strings.Contains(strings.ToUpper(string(data[loc[0]:end])), "PING") {
							replies.WriteString("+PONG\r\n")
						} else {
							replies.WriteString("-ERR unknown command\r\n")
						}
					}
					if _, err := c.Write([]byte(replies.String())); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func newRedisStateTestStorages(t *testing.T) (logging.LoggerInterface, *storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl) {
	t.Helper()
	logger := logging.NewLogger(nil)
	db, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return logger, storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false),
		storage.NewProxyRuleBasedSegmentsStorage(db, logger, false), storage.NewProxySegmentStorage(db, logger, false)
}

func TestSetupRedisStateStartFailureIsInitError(t *testing.T) {
	t.Run("client factory failure", func(t *testing.T) {
		original := newRedisStateClient
		t.Cleanup(func() { newRedisStateClient = original })
		newRedisStateClient = func(*commonconf.RedisConfig, logging.LoggerInterface) (*redis.PrefixedRedisClient, error) {
			return nil, errors.New("injected client factory failure")
		}

		logger, splits, rbs, segments := newRedisStateTestStorages(t)
		cfg := redisStateTestConfig(t)
		writer, err := setupRedisState(cfg, logger, splits, rbs, segments)
		assert.True(t, writer == nil)
		assertInitError(t, err, common.ExitRedisInitializationFailed)
		if err != nil {
			assert.Contains(t, err.Error(), "injected client factory failure")
		}
	})

	t.Run("start probe failure is returned unwrapped", func(t *testing.T) {
		logger, splits, rbs, segments := newRedisStateTestStorages(t)
		cfg := redisStateTestConfig(t)
		cfg.Storage.Redis.Connection.Host = "127.0.0.1"
		cfg.Storage.Redis.Connection.Port = fakeRedisProbeFailureServer(t)

		writer, err := setupRedisState(cfg, logger, splits, rbs, segments)
		assert.True(t, writer == nil)
		initErr, ok := err.(*common.InitializationError)
		if !assert.True(t, ok, "start failure must be returned unwrapped, got %T: %v", err, err) {
			return
		}
		assert.Equal(t, common.ExitRedisInitializationFailed, initErr.ExitCode())
		assert.Contains(t, err.Error(), "requires Redis 5.0 or later")
	})
}

func TestSetupRedisStateUnreachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := redisStateTestConfig(t)
	cfg.Storage.Redis.Connection.Host = "127.0.0.1"
	cfg.Storage.Redis.Connection.Port = port

	writer, err := setupRedisState(cfg, logging.NewLogger(nil), nil, nil, nil)
	assert.True(t, writer == nil)
	assertInitError(t, err, common.ExitRedisInitializationFailed)
}

func TestSetupRedisStateWiresStorages(t *testing.T) {
	logger := logging.NewLogger(nil)
	db, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	splits := storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := storage.NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segments := storage.NewProxySegmentStorage(db, logger, false)

	cfg := redisStateTestConfig(t)
	writer, err := setupRedisState(cfg, logger, splits, rbs, segments)
	if err != nil {
		t.Fatal(err)
	}
	if writer == nil {
		t.Fatal("expected a redis state writer when enabled")
	}
	t.Cleanup(func() {
		writer.Stop()
		if err := writer.Reset(); err != nil {
			t.Errorf("error cleaning up redis state: %v", err)
		}
	})

	splits.Update([]dtos.SplitDTO{{Name: "wired_flag"}}, nil, 10)
	if err := rbs.Update([]dtos.RuleBasedSegmentDTO{{Name: "wired_rbs"}}, nil, 10); err != nil {
		t.Fatal(err)
	}
	if err := segments.Update("wired_segment", set.NewSet("k1"), set.NewSet(), 10); err != nil {
		t.Fatal(err)
	}
	waitForRedisStateFlush(t, writer)
	assert.True(t, writer.Status().Healthy)

	opts, err := common.ParseRedisOptions(&cfg.Storage.Redis.Connection)
	if err != nil {
		t.Fatal(err)
	}
	client, err := commonredis.NewRedisClient(opts, logger)
	if err != nil {
		t.Fatal(err)
	}
	prefix := client.Prefix()
	keys, err := redisstate.NewKeys(prefix, cfg.Apikey, "")
	if err != nil {
		t.Fatal(err)
	}

	for name, group := range map[string][]string{
		"flags":               keys.FlagKeys(),
		"rule based segments": keys.RuleBasedSegmentKeys(),
		"segment":             keys.SegmentKeys("wired_segment"),
	} {
		n, err := client.Exists(unprefixedKeys(prefix, group)...)
		assert.NoError(t, err, name)
		assert.Greater(t, n, int64(0), name)
	}
}

func TestStartRejectsRedisStateInOfflineMode(t *testing.T) {
	cfg := redisStateTestConfig(t)
	cfg.Offline = true

	err := Start(logging.NewLogger(nil), cfg)
	assertInitError(t, err, common.ExitInvalidConfiguration)
}

func TestSetupRedisStateStopOnShutdown(t *testing.T) {
	logger := logging.NewLogger(nil)
	db, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	splits := storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := storage.NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segments := storage.NewProxySegmentStorage(db, logger, false)

	cfg := redisStateTestConfig(t)
	writer, err := setupRedisState(cfg, logger, splits, rbs, segments)
	if err != nil {
		t.Fatal(err)
	}
	if writer == nil {
		t.Fatal("expected a redis state writer when enabled")
	}
	t.Cleanup(func() {
		if err := writer.Reset(); err != nil {
			t.Errorf("error cleaning up redis state: %v", err)
		}
	})

	assert.True(t, stopRedisState(writer, logger, 10*time.Second))

	// A second stop must return immediately and not hang on the already-closed worker.
	assert.True(t, stopRedisState(writer, logger, 10*time.Second))
	assert.True(t, stopRedisState(nil, logger, time.Second))
}

func TestMaskedConfigForAdminMasksSecrets(t *testing.T) {
	const apikey = "placeholder-test-apikey"
	const password = "supersecret"

	cfg := &pconf.Main{}
	cfg.Apikey = apikey
	cfg.Storage.Redis.Connection.Pass = password

	masked := maskedConfigForAdmin(cfg)

	if masked.Apikey == apikey {
		t.Fatalf("expected obfuscated apikey, got the original value")
	}
	if strings.Contains(masked.Apikey, apikey) {
		t.Errorf("masked apikey %q still contains the original apikey", masked.Apikey)
	}
	if strings.Contains(fmt.Sprintf("%+v", masked), password) {
		t.Errorf("masked config still contains the redis password")
	}
	if masked.Storage.Redis.Connection.Pass != adminRedisPassMask {
		t.Errorf("expected pass %q, got %q", adminRedisPassMask, masked.Storage.Redis.Connection.Pass)
	}

	if cfg.Apikey != apikey {
		t.Errorf("original apikey was mutated: %q", cfg.Apikey)
	}
	if cfg.Storage.Redis.Connection.Pass != password {
		t.Errorf("original redis password was mutated: %q", cfg.Storage.Redis.Connection.Pass)
	}
}

// stuckClient blocks HGetAll, so the writer worker never gets past the startup resync
type stuckClient struct {
	entered chan struct{}
	release chan struct{}
}

func (c *stuckClient) HGetAll(string) (map[string]string, error) {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-c.release
	return map[string]string{}, nil
}

func (c *stuckClient) Eval(string, []string, ...interface{}) error { return nil }
func (c *stuckClient) Prefix() string                              { return "" }
func (c *stuckClient) Scan(uint64, string, int64) ([]string, uint64, error) {
	return nil, 0, nil
}
func (c *stuckClient) Del(...string) (int64, error) { return 0, nil }
func (c *stuckClient) Get(string) (string, error)   { return "", nil }

type emptyRedisStateSource struct{}

func (emptyRedisStateSource) Flags() ([]dtos.SplitDTO, int64, error) { return nil, 0, nil }
func (emptyRedisStateSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	return nil, 0, nil
}
func (emptyRedisStateSource) Segments(func(redisstate.SegmentState) error) error { return nil }

// warnLogger records Warning lines
type warnLogger struct {
	logging.LoggerInterface
	mtx      sync.Mutex
	warnings []string
}

func (l *warnLogger) Warning(msg ...interface{}) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.warnings = append(l.warnings, fmt.Sprint(msg...))
}

func (l *warnLogger) warned() []string {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return append([]string(nil), l.warnings...)
}

func TestStopRedisStateTimeout(t *testing.T) {
	client := &stuckClient{entered: make(chan struct{}, 1), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(client.release) }) }
	t.Cleanup(release)

	logger := &warnLogger{LoggerInterface: logging.NewLogger(nil)}
	keys, err := redisstate.NewKeys("t-"+t.Name(), "apikey-test", "")
	assert.NoError(t, err)
	writer, err := redisstate.NewWriter(client, keys, "fp-test", redisstate.DefaultOptions(), logger)
	assert.NoError(t, err)
	writer.Start(emptyRedisStateSource{})
	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the writer never reached Redis")
	}

	stopped := make(chan bool, 1)
	go func() { stopped <- stopRedisState(writer, logger, 50*time.Millisecond) }()
	select {
	case ok := <-stopped:
		assert.False(t, ok, "the stuck writer does not stop in time")
	case <-time.After(5 * time.Second):
		t.Fatal("stopRedisState blocked on the stuck writer instead of timing out")
	}
	warnings := logger.warned()
	if assert.Len(t, warnings, 1) {
		assert.Contains(t, warnings[0], "did not stop within")
	}

	release()
	assert.True(t, stopRedisState(writer, logger, 5*time.Second), "once Redis answers, the writer stops")
}

func TestStopWaitBound(t *testing.T) {
	cases := []struct {
		name  string
		state pconf.RedisState
		conn  conf.Redis
		want  time.Duration
	}{
		{
			name:  "defaults with no retries",
			state: pconf.RedisState{StopTimeoutMs: 10000},
			conn:  conf.Redis{DialTimeout: 5, ReadTimeout: 10, WriteTimeout: 5, MaxRetries: 0},
			want:  35 * time.Second,
		},
		{
			name:  "two retries multiply the per-call timeouts",
			state: pconf.RedisState{StopTimeoutMs: 1000},
			conn:  conf.Redis{DialTimeout: 1, ReadTimeout: 2, WriteTimeout: 3, MaxRetries: 2},
			want:  24 * time.Second,
		},
		{
			name:  "zero connection timeouts leave the stop timeout plus the margin",
			state: pconf.RedisState{StopTimeoutMs: 60000},
			conn:  conf.Redis{MaxRetries: 0},
			want:  65 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stopWaitBound(tc.state, tc.conn))
		})
	}
}

// evalBlockClient lets the startup probe pass, then blocks every Eval until release once armed, so the writer
// worker is stuck inside a Redis call.
type evalBlockClient struct {
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (c *evalBlockClient) Eval(string, []string, ...interface{}) error {
	if !c.armed.Load() {
		return nil
	}
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-c.release
	return nil
}
func (c *evalBlockClient) HGetAll(string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (c *evalBlockClient) Prefix() string { return "" }
func (c *evalBlockClient) Scan(uint64, string, int64) ([]string, uint64, error) {
	return nil, 0, nil
}
func (c *evalBlockClient) Del(...string) (int64, error) { return 0, nil }
func (c *evalBlockClient) Get(string) (string, error)   { return "", nil }

func TestStopNotBlockedByHungEval(t *testing.T) {
	client := &evalBlockClient{entered: make(chan struct{}, 1), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(client.release) }) }
	t.Cleanup(release)

	logger := &warnLogger{LoggerInterface: logging.NewLogger(nil)}
	keys, err := redisstate.NewKeys("t-"+t.Name(), "apikey-test", "")
	assert.NoError(t, err)
	writer, err := redisstate.NewWriter(client, keys, "fp-test", redisstate.DefaultOptions(), logger)
	assert.NoError(t, err)
	assert.NoError(t, writer.Start(emptyRedisStateSource{}))
	client.armed.Store(true)
	writer.PutFlags([]dtos.SplitDTO{{Name: "hung-eval-flag", ChangeNumber: 1}}, nil, 1)
	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the writer never reached Redis")
	}

	// A short bound keeps the test fast. The production bound is covered by TestStopWaitBound.
	stopped := make(chan bool, 1)
	go func() { stopped <- stopRedisState(writer, logger, 50*time.Millisecond) }()
	select {
	case ok := <-stopped:
		assert.False(t, ok, "a hung Eval does not stop within the bound")
	case <-time.After(5 * time.Second):
		t.Fatal("stopRedisState blocked on the hung Eval instead of returning at the bound")
	}
	warnings := logger.warned()
	if assert.Len(t, warnings, 1) {
		assert.Contains(t, warnings[0], "did not stop within")
	}

	release()
	done := make(chan struct{})
	go func() {
		writer.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the writer worker did not exit after the Eval was released")
	}
}
