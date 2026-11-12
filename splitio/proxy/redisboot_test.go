package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

const bootTestPrefix = "bootprefix:"

type bootLogEntry struct {
	level string
	msg   string
}

type bootTestLogger struct {
	entries []bootLogEntry
}

func (l *bootTestLogger) add(level string, msg ...interface{}) {
	l.entries = append(l.entries, bootLogEntry{level: level, msg: fmt.Sprint(msg...)})
}

func (l *bootTestLogger) Error(msg ...interface{})   { l.add("error", msg...) }
func (l *bootTestLogger) Warning(msg ...interface{}) { l.add("warning", msg...) }
func (l *bootTestLogger) Info(msg ...interface{})    { l.add("info", msg...) }
func (l *bootTestLogger) Debug(msg ...interface{})   { l.add("debug", msg...) }
func (l *bootTestLogger) Verbose(msg ...interface{}) { l.add("verbose", msg...) }

func (l *bootTestLogger) has(level, substr string) bool {
	for _, e := range l.entries {
		if e.level == level && strings.Contains(e.msg, substr) {
			return true
		}
	}
	return false
}

// bootFakeClient answers the reader from an in-memory meta hash. Every other key is empty.
type bootFakeClient struct {
	keys   redisstate.Keys
	meta   map[string]string
	err    error
	closes int
}

func (f *bootFakeClient) Prefix() string { return bootTestPrefix }

func (f *bootFakeClient) HGetAll(_ context.Context, key string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if key == f.keys.Meta() {
		return f.meta, nil
	}
	return map[string]string{}, nil
}

func (f *bootFakeClient) ZScan(_ context.Context, _ string, _ uint64, _ int64) ([]string, uint64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return nil, 0, nil
}

func (f *bootFakeClient) ZCard(_ context.Context, _ string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return 0, nil
}

func (f *bootFakeClient) HGet(_ context.Context, key, field string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	if key != f.keys.Meta() {
		return "", false, nil
	}
	v, ok := f.meta[field]
	return v, ok, nil
}

func (f *bootFakeClient) Close() error {
	f.closes++
	return nil
}

func bootTestConfig(snapshot string) *pconf.Main {
	cfg := &pconf.Main{
		Apikey:          "apikey-boot-test",
		FlagSpecVersion: "1.2",
		Storage: pconf.Storage{Redis: pconf.RedisState{
			Enabled:           true,
			Namespace:         "boot",
			ChunkSize:         1000,
			PurgeBatch:        500,
			StopTimeoutMs:     10000,
			MaxPendingChanges: 3000000,
			WriteMaxAttempts:  3,
			WriteBackoffMaxMs: 60000,
			RetentionHours:    24,
			Connection:        conf.Redis{Host: "localhost", Port: 6379},
		}},
	}
	cfg.Initialization.Snapshot = snapshot
	return cfg
}

func bootTestKeys(t *testing.T, cfg *pconf.Main) redisstate.Keys {
	t.Helper()
	keys, err := redisstate.NewKeys(bootTestPrefix, cfg.Apikey, cfg.Storage.Redis.Namespace)
	require.NoError(t, err)
	return keys
}

func bootTestFingerprint(cfg *pconf.Main) string {
	return util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
}

// bootValidMeta has matching cursors and fingerprint but no entities, so the reader reports Valid.
func bootValidMeta(cfg *pconf.Main, keys redisstate.Keys) map[string]string {
	return map[string]string{
		redisstate.FingerprintField:        bootTestFingerprint(cfg),
		keys.FlagCursorField():             "10",
		keys.RuleBasedSegmentCursorField(): "10",
	}
}

func bootForeignMeta(_ *pconf.Main, keys redisstate.Keys) map[string]string {
	return map[string]string{
		redisstate.FingerprintField: "other-environment",
		keys.FlagCursorField():      "10",
	}
}

func bootCorruptMeta(_ *pconf.Main, keys redisstate.Keys) map[string]string {
	return map[string]string{
		keys.FlagCursorField(): "1",
	}
}

// bootOpener returns an opener that hands out the fake and counts its calls.
func bootOpener(client *bootFakeClient, calls *int) bootClientOpener {
	return func(*pconf.Main) (bootReadClient, error) {
		*calls++
		return client, nil
	}
}

// bootNoOpener fails the test if the read client is built.
func bootNoOpener(t *testing.T) bootClientOpener {
	return func(*pconf.Main) (bootReadClient, error) {
		t.Fatal("the read client must not be built")
		return nil, nil
	}
}

func requireBootExitCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var initErr *common.InitializationError
	require.True(t, errors.As(err, &initErr), "error is not an InitializationError: %v", err)
	assert.Equal(t, code, initErr.ExitCode())
}

// Boot decision for each state without entities (absent, corrupt, foreign, unreachable, valid-but-empty).
// Populated states are in TestPopulatedValidAndStale; cloud up/down is covered at Start level.
func TestBootMatrix(t *testing.T) {
	type matrixCase struct {
		name       string
		snapshot   string
		meta       func(*pconf.Main, redisstate.Keys) map[string]string
		readErr    error
		wantStatus redisstate.BootStatus
		wantSource BootSource
		wantLevel  string
		wantCode   int
	}
	cases := []matrixCase{
		{name: "absent", wantStatus: redisstate.BootAbsent, wantSource: BootSourceCold, wantLevel: "warning"},
		{name: "absent with snapshot file", snapshot: "/tmp/snapshot.json", wantStatus: redisstate.BootAbsent, wantSource: BootSourceSnapshotFile, wantLevel: "warning"},
		{name: "valid but empty", meta: bootValidMeta, wantStatus: redisstate.BootValid, wantSource: BootSourceCold, wantLevel: "warning"},
		{name: "corrupt", meta: bootCorruptMeta, wantStatus: redisstate.BootCorrupt, wantSource: BootSourceCold, wantLevel: "warning"},
		{name: "foreign", meta: bootForeignMeta, wantStatus: redisstate.BootForeign, wantSource: BootSourceCold, wantLevel: "warning"},
		{name: "unreachable", readErr: errors.New("dial tcp 127.0.0.1:6379: connect: connection refused"), wantCode: common.ExitRedisInitializationFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := bootTestConfig(c.snapshot)
			keys := bootTestKeys(t, cfg)
			var meta map[string]string
			if c.meta != nil {
				meta = c.meta(cfg, keys)
			}
			client := &bootFakeClient{keys: keys, meta: meta, err: c.readErr}
			calls := 0
			logger := &bootTestLogger{}

			decision, err := resolveRedisBoot(cfg, logger, bootOpener(client, &calls))

			assert.Equal(t, 1, client.closes, "read client must be closed once (T33)")
			assert.Equal(t, 1, calls)
			if c.wantCode != 0 {
				requireBootExitCode(t, err, c.wantCode)
				assert.Empty(t, decision.Source)
				return
			}
			require.NoError(t, err)
			assert.NotEqual(t, BootSourceRedis, decision.Source, "a non-usable redis state is never served")
			assert.Equal(t, c.wantSource, decision.Source)
			require.NotNil(t, decision.State)
			assert.Equal(t, c.wantStatus, decision.State.Status)
			assert.True(t, logger.has(c.wantLevel, ""), "expected a %s log", c.wantLevel)
		})
	}
}

// Populated valid state: see TestPopulatedValidAndStale.

// Force-fresh-startup skips the Redis read.
func TestForceFreshStartupSkipsRead(t *testing.T) {
	cfg := bootTestConfig("")
	cfg.Initialization.ForceFreshStartup = true
	logger := &bootTestLogger{}

	decision, err := resolveRedisBoot(cfg, logger, bootNoOpener(t))

	require.NoError(t, err)
	assert.Equal(t, BootSourceCold, decision.Source)
	assert.Nil(t, decision.State)
	assert.True(t, logger.has("info", "force-fresh-startup"))
}

// Redis state disabled means no Redis read at all.
func TestRedisDisabledSkipsRead(t *testing.T) {
	cfg := bootTestConfig("/tmp/snapshot.json")
	cfg.Storage.Redis.Enabled = false

	decision, err := resolveRedisBoot(cfg, &bootTestLogger{}, bootNoOpener(t))

	require.NoError(t, err)
	assert.Equal(t, BootSourceSnapshotFile, decision.Source)
	assert.Nil(t, decision.State)
}

// Unreachable redis exits ExitRedisInitializationFailed; bad settings exit ExitInvalidConfiguration.
func TestBuildAndReadErrorCodes(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		cfg := bootTestConfig("")
		client := &bootFakeClient{keys: bootTestKeys(t, cfg), err: errors.New("connection refused")}
		calls := 0
		_, err := resolveRedisBoot(cfg, &bootTestLogger{}, bootOpener(client, &calls))
		requireBootExitCode(t, err, common.ExitRedisInitializationFailed)
		assert.Equal(t, 1, client.closes)
	})

	t.Run("opener reports bad config", func(t *testing.T) {
		cfg := bootTestConfig("")
		opener := func(*pconf.Main) (bootReadClient, error) {
			return nil, common.NewInitError(errors.New("bad connection options"), common.ExitInvalidConfiguration)
		}
		_, err := resolveRedisBoot(cfg, &bootTestLogger{}, opener)
		requireBootExitCode(t, err, common.ExitInvalidConfiguration)
	})

	t.Run("invalid settings never build the client", func(t *testing.T) {
		cfg := bootTestConfig("")
		cfg.Storage.Redis.ChunkSize = 0
		_, err := resolveRedisBoot(cfg, &bootTestLogger{}, bootNoOpener(t))
		requireBootExitCode(t, err, common.ExitInvalidConfiguration)
	})
}

// Config errors map to ExitInvalidConfiguration. Connection failures surface on the first read, not at build time.
func TestOpenRedisBootReadClient(t *testing.T) {
	t.Run("missing TLS CA file is invalid configuration", func(t *testing.T) {
		cfg := bootTestConfig("")
		cfg.Storage.Redis.Connection.TLS = true
		cfg.Storage.Redis.Connection.TLSCACertificates = []string{filepath.Join(t.TempDir(), "missing-ca.pem")}

		client, err := openRedisBootReadClient(cfg)

		requireBootExitCode(t, err, common.ExitInvalidConfiguration)
		assert.Nil(t, client)
	})

	t.Run("sentinel without master name is invalid configuration in the read client builder", func(t *testing.T) {
		cfg := bootTestConfig("")
		cfg.Storage.Redis.Connection.SentinelReplication = true
		cfg.Storage.Redis.Connection.SentinelAddresses = "127.0.0.1:26379"

		client, err := openRedisBootReadClient(cfg)

		requireBootExitCode(t, err, common.ExitInvalidConfiguration)
		assert.Nil(t, client)
	})

	t.Run("unreachable address builds a client without connecting", func(t *testing.T) {
		cfg := bootTestConfig("")
		cfg.Storage.Redis.Connection.Host = "127.0.0.1"
		cfg.Storage.Redis.Connection.Port = 1
		cfg.Storage.Redis.Connection.Prefix = bootTestPrefix

		client, err := openRedisBootReadClient(cfg)

		require.NoError(t, err)
		require.NotNil(t, client)
		assert.Equal(t, bootTestPrefix, client.Prefix())
		assert.NoError(t, client.Close())
	})

	t.Run("unreachable redis is fatal at the first read", func(t *testing.T) {
		cfg := bootTestConfig("")
		cfg.Storage.Redis.Connection.Host = "127.0.0.1"
		cfg.Storage.Redis.Connection.Port = 1
		cfg.Storage.Redis.Connection.Prefix = bootTestPrefix
		cfg.Storage.Redis.Connection.DialTimeout = 1
		cfg.Storage.Redis.Connection.ReadTimeout = 1

		_, err := resolveRedisBoot(cfg, &bootTestLogger{}, openRedisBootReadClient)

		requireBootExitCode(t, err, common.ExitRedisInitializationFailed)
	})
}

// The foreign warning names the namespace, points to force-fresh-startup and never serves the state.
func TestForeignWarningContent(t *testing.T) {
	cfg := bootTestConfig("")
	keys := bootTestKeys(t, cfg)
	client := &bootFakeClient{keys: keys, meta: bootForeignMeta(cfg, keys)}
	calls := 0
	logger := &bootTestLogger{}

	decision, err := resolveRedisBoot(cfg, logger, bootOpener(client, &calls))

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.NotEqual(t, BootSourceRedis, decision.Source)
	assert.True(t, logger.has("warning", "namespace boot"))
	assert.True(t, logger.has("warning", "force-fresh-startup"))
	assert.False(t, logger.has("info", "starting from redis state"))
}

// A valid state with no flags, rule-based segments or segments is treated as absent and never served.
func TestValidEmptyTreatedAsAbsent(t *testing.T) {
	cfg := bootTestConfig("")
	keys := bootTestKeys(t, cfg)
	client := &bootFakeClient{keys: keys, meta: bootValidMeta(cfg, keys)}
	calls := 0
	logger := &bootTestLogger{}

	decision, err := resolveRedisBoot(cfg, logger, bootOpener(client, &calls))

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	require.NotNil(t, decision.State)
	assert.Equal(t, redisstate.BootValid, decision.State.Status)
	assert.Equal(t, BootSourceCold, decision.Source)
	assert.True(t, logger.has("warning", "treated as absent"))
}

// A cursor without a fingerprint is corrupt and restores nothing.
func TestCorruptMissingFingerprintRestoresNothing(t *testing.T) {
	cfg := bootTestConfig("")
	keys := bootTestKeys(t, cfg)
	client := &bootFakeClient{keys: keys, meta: bootCorruptMeta(cfg, keys)}
	calls := 0

	decision, err := resolveRedisBoot(cfg, &bootTestLogger{}, bootOpener(client, &calls))
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, BootSourceCold, decision.Source)

	cursorCalls := 0
	apply := applyCommittedCursors
	applyCommittedCursors = func(redisstate.BootState, *storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl) error {
		cursorCalls++
		return nil
	}
	t.Cleanup(func() { applyCommittedCursors = apply })

	require.NoError(t, applyRedisCursors(decision, nil, nil, nil))
	assert.Equal(t, 0, cursorCalls)
}

// A cursor failure returns ExitRedisInitializationFailed; other sources are a no-op.
func TestCursorFailuresAreInitErrors(t *testing.T) {
	apply := applyCommittedCursors
	t.Cleanup(func() { applyCommittedCursors = apply })

	state := redisstate.BootState{Status: redisstate.BootValid}
	boot := BootDecision{Source: BootSourceRedis, State: &state}
	boom := errors.New("bolt write failed")

	t.Run("cursors fail", func(t *testing.T) {
		applyCommittedCursors = func(redisstate.BootState, *storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl) error {
			return boom
		}
		err := applyRedisCursors(boot, nil, nil, nil)
		requireBootExitCode(t, err, common.ExitRedisInitializationFailed)
		assert.Contains(t, err.Error(), boom.Error())
	})

	t.Run("success applies once", func(t *testing.T) {
		calls := 0
		applyCommittedCursors = func(redisstate.BootState, *storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl) error {
			calls++
			return nil
		}
		require.NoError(t, applyRedisCursors(boot, nil, nil, nil))
		assert.Equal(t, 1, calls)
	})

	t.Run("non redis source is a no-op", func(t *testing.T) {
		applyCommittedCursors = func(redisstate.BootState, *storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl) error {
			return boom
		}
		require.NoError(t, applyRedisCursors(BootDecision{Source: BootSourceCold}, nil, nil, nil))
	})
}

// bootPopulatedClient serves a full Redis state from memory. Sorted sets are flat member, score pairs.
type bootPopulatedClient struct {
	hashes map[string]map[string]string
	zsets  map[string][]string
	closes int
}

func (f *bootPopulatedClient) Prefix() string { return bootTestPrefix }

func (f *bootPopulatedClient) HGetAll(_ context.Context, key string) (map[string]string, error) {
	out := map[string]string{}
	for field, value := range f.hashes[key] {
		out[field] = value
	}
	return out, nil
}

func (f *bootPopulatedClient) ZScan(_ context.Context, key string, _ uint64, _ int64) ([]string, uint64, error) {
	return f.zsets[key], 0, nil
}

func (f *bootPopulatedClient) HGet(_ context.Context, key, field string) (string, bool, error) {
	v, ok := f.hashes[key][field]
	return v, ok, nil
}

func (f *bootPopulatedClient) Close() error {
	f.closes++
	return nil
}

func (f *bootPopulatedClient) setScores(key string, members map[string]int64) {
	for member, score := range members {
		f.zsets[key] = append(f.zsets[key], member, fmt.Sprint(score))
	}
}

// bootClosesOf returns how many times the fake read client was closed.
func bootClosesOf(c bootReadClient) int {
	switch v := c.(type) {
	case *bootFakeClient:
		return v.closes
	case *bootPopulatedClient:
		return v.closes
	}
	return -1
}

func bootSegmentMatchers(segments []string) []dtos.MatcherDTO {
	out := make([]dtos.MatcherDTO, 0, len(segments))
	for _, name := range segments {
		out = append(out, dtos.MatcherDTO{UserDefinedSegment: &dtos.UserDefinedSegmentMatcherDataDTO{SegmentName: name}})
	}
	return out
}

func bootFlagJSON(t *testing.T, name string, changeNumber int64, segments ...string) string {
	t.Helper()
	d := dtos.SplitDTO{Name: name, Status: "ACTIVE", ChangeNumber: changeNumber}
	if len(segments) > 0 {
		d.Conditions = []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: bootSegmentMatchers(segments)}}}
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return string(b)
}

func bootRbsJSON(t *testing.T, name string, changeNumber int64, segments ...string) string {
	t.Helper()
	d := dtos.RuleBasedSegmentDTO{Name: name, Status: "ACTIVE", ChangeNumber: changeNumber}
	if len(segments) > 0 {
		d.Conditions = []dtos.RuleBasedConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: bootSegmentMatchers(segments)}}}
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return string(b)
}

// bootPopulatedRedis builds a valid state: flags f1 (uses s1) and f2, rule-based r1 (uses s2), segments s1 and s2.
// Each cursor trails its change number by lag.
func bootPopulatedRedis(t *testing.T, cfg *pconf.Main, lag int64) *bootPopulatedClient {
	t.Helper()
	keys := bootTestKeys(t, cfg)
	f := &bootPopulatedClient{hashes: map[string]map[string]string{}, zsets: map[string][]string{}}
	f.hashes[keys.Meta()] = map[string]string{
		redisstate.FingerprintField:        bootTestFingerprint(cfg),
		keys.FlagCursorField():             fmt.Sprint(10 - lag),
		keys.RuleBasedSegmentCursorField(): fmt.Sprint(20 - lag),
		keys.SegmentCursorField("s1"):      fmt.Sprint(30 - lag),
		keys.SegmentCursorField("s2"):      fmt.Sprint(31 - lag),
	}
	f.hashes[keys.FlagData()] = map[string]string{
		"f1": bootFlagJSON(t, "f1", 10, "s1"),
		"f2": bootFlagJSON(t, "f2", 9),
	}
	f.setScores(keys.FlagIndex(), map[string]int64{"f1": 10, "f2": 9})
	f.hashes[keys.RuleBasedSegmentData()] = map[string]string{"r1": bootRbsJSON(t, "r1", 20, "s2")}
	f.setScores(keys.RuleBasedSegmentIndex(), map[string]int64{"r1": 20})
	f.setScores(keys.SegmentActive("s1"), map[string]int64{"k1": 30, "k2": 25})
	f.setScores(keys.SegmentTombstones("s1"), map[string]int64{"k3": 28})
	f.setScores(keys.SegmentActive("s2"), map[string]int64{"k9": 31})
	return f
}

// A populated Redis state is served when in sync and when cursors lag (stale). The snapshot is never read.
func TestPopulatedValidAndStale(t *testing.T) {
	cases := []struct {
		name       string
		lag        int64
		wantCursor int64
	}{
		{name: "valid", lag: 0, wantCursor: 10},
		{name: "stale cursor behind", lag: 5, wantCursor: 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := bootTestConfig("/tmp/snapshot.json")
			client := bootPopulatedRedis(t, cfg, c.lag)

			decision, err := resolveRedisBoot(cfg, &bootTestLogger{}, func(*pconf.Main) (bootReadClient, error) { return client, nil })

			require.NoError(t, err)
			assert.Equal(t, 1, client.closes, "read client must be closed once")
			assert.Equal(t, BootSourceRedis, decision.Source)
			require.NotNil(t, decision.State)
			assert.Equal(t, redisstate.BootValid, decision.State.Status)
			assert.Equal(t, c.wantCursor, decision.State.FlagCursor)
			assert.Len(t, decision.State.Flags, 2)
			assert.Len(t, decision.State.RuleBasedSegments, 1)
			assert.Len(t, decision.State.Segments, 2)
		})
	}
}

// The read client is closed exactly once on every decision path.
func TestClientClosedOnEveryDecisionPath(t *testing.T) {
	cases := []struct {
		name       string
		client     func(t *testing.T, cfg *pconf.Main, keys redisstate.Keys) bootReadClient
		wantErr    bool
		wantSource BootSource
	}{
		{name: "valid", client: func(t *testing.T, cfg *pconf.Main, _ redisstate.Keys) bootReadClient {
			return bootPopulatedRedis(t, cfg, 0)
		}, wantSource: BootSourceRedis},
		{name: "corrupt", client: func(_ *testing.T, cfg *pconf.Main, keys redisstate.Keys) bootReadClient {
			return &bootFakeClient{keys: keys, meta: bootCorruptMeta(cfg, keys)}
		}, wantSource: BootSourceCold},
		{name: "foreign", client: func(_ *testing.T, cfg *pconf.Main, keys redisstate.Keys) bootReadClient {
			return &bootFakeClient{keys: keys, meta: bootForeignMeta(cfg, keys)}
		}, wantSource: BootSourceCold},
		{name: "absent", client: func(_ *testing.T, _ *pconf.Main, keys redisstate.Keys) bootReadClient {
			return &bootFakeClient{keys: keys}
		}, wantSource: BootSourceCold},
		{name: "unreachable", client: func(_ *testing.T, _ *pconf.Main, keys redisstate.Keys) bootReadClient {
			return &bootFakeClient{keys: keys, err: errors.New("connection refused")}
		}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := bootTestConfig("")
			client := c.client(t, cfg, bootTestKeys(t, cfg))

			decision, err := resolveRedisBoot(cfg, &bootTestLogger{}, func(*pconf.Main) (bootReadClient, error) { return client, nil })

			assert.Equal(t, 1, bootClosesOf(client), "read client must be closed exactly once (T33)")
			if c.wantErr {
				requireBootExitCode(t, err, common.ExitRedisInitializationFailed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.wantSource, decision.Source)
		})
	}
}

// bootRecordingSink records what the reader hands over; failOn makes the named step return an error.
type bootRecordingSink struct {
	flags, rbs int
	segments   []string
	failOn     string
	err        error
}

func (k *bootRecordingSink) Entities(flags []dtos.SplitDTO, rbs []dtos.RuleBasedSegmentDTO) error {
	if k.failOn == "entities" {
		return k.err
	}
	k.flags, k.rbs = len(flags), len(rbs)
	return nil
}

func (k *bootRecordingSink) Segment(st redisstate.SegmentState) error {
	if k.failOn == "segment" {
		return k.err
	}
	k.segments = append(k.segments, st.Name)
	return nil
}

// With a sink the boot state holds no segment payloads, only the count, and the sink got every segment.
func TestBootStateHoldsNoSegmentPayloadsWithSink(t *testing.T) {
	cfg := bootTestConfig("")
	client := bootPopulatedRedis(t, cfg, 0)
	sink := &bootRecordingSink{}

	decision, err := resolveRedisBootInto(cfg, &bootTestLogger{}, func(*pconf.Main) (bootReadClient, error) { return client, nil }, sink)

	require.NoError(t, err)
	require.Equal(t, BootSourceRedis, decision.Source)
	assert.Empty(t, decision.State.Segments)
	assert.Equal(t, 2, decision.State.SegmentCount)
	assert.ElementsMatch(t, []string{"s1", "s2"}, sink.segments)
	assert.Equal(t, 2, sink.flags)
	assert.Equal(t, 1, sink.rbs)
}

// A sink failure is ExitRedisInitializationFailed and the read client stays closed.
func TestSinkFailureIsInitErrorAndClientClosed(t *testing.T) {
	boom := errors.New("bolt write failed")
	for _, failOn := range []string{"entities", "segment"} {
		t.Run(failOn, func(t *testing.T) {
			cfg := bootTestConfig("")
			client := bootPopulatedRedis(t, cfg, 0)

			_, err := resolveRedisBootInto(cfg, &bootTestLogger{}, func(*pconf.Main) (bootReadClient, error) { return client, nil },
				&bootRecordingSink{failOn: failOn, err: boom})

			requireBootExitCode(t, err, common.ExitRedisInitializationFailed)
			assert.Contains(t, err.Error(), boom.Error())
			assert.Equal(t, 1, client.closes)
		})
	}
}

// Bad redis settings exit with ExitInvalidConfiguration and never build a client.
func TestBadSettingsNeverBuildClient(t *testing.T) {
	cases := map[string]func(cfg *pconf.Main){
		"chunk size zero": func(cfg *pconf.Main) { cfg.Storage.Redis.ChunkSize = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := bootTestConfig("")
			mutate(cfg)
			_, err := resolveRedisBoot(cfg, &bootTestLogger{}, bootNoOpener(t))
			requireBootExitCode(t, err, common.ExitInvalidConfiguration)
		})
	}
}

// A flags index/data mismatch is corrupt; nothing is restored and the source falls back.
func TestFlagsIndexDataMismatchIsCorrupt(t *testing.T) {
	cases := []struct {
		name     string
		snapshot string
		mutate   func(t *testing.T, client *bootPopulatedClient, keys redisstate.Keys)
		wantSrc  BootSource
	}{
		{
			name: "index lacks a flag held in data",
			mutate: func(_ *testing.T, client *bootPopulatedClient, keys redisstate.Keys) {
				client.zsets[keys.FlagIndex()] = []string{"f1", "10"}
			},
			wantSrc: BootSourceCold,
		},
		{
			name:     "data entry missing from index",
			snapshot: "/tmp/snapshot.json",
			mutate: func(t *testing.T, client *bootPopulatedClient, keys redisstate.Keys) {
				data := client.hashes[keys.FlagData()]
				delete(data, "f2")
				data["f3"] = bootFlagJSON(t, "f3", 9)
			},
			wantSrc: BootSourceSnapshotFile,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := bootTestConfig(c.snapshot)
			keys := bootTestKeys(t, cfg)
			client := bootPopulatedRedis(t, cfg, 0)
			c.mutate(t, client, keys)
			logger := &bootTestLogger{}

			decision, err := resolveRedisBoot(cfg, logger, func(*pconf.Main) (bootReadClient, error) { return client, nil })

			require.NoError(t, err)
			assert.Equal(t, 1, client.closes)
			assert.Equal(t, c.wantSrc, decision.Source)
			require.NotNil(t, decision.State)
			assert.Equal(t, redisstate.BootCorrupt, decision.State.Status)
			assert.Empty(t, decision.State.Flags)
			assert.Empty(t, decision.State.RuleBasedSegments)
			assert.True(t, logger.has("warning", "corrupt"))
		})
	}
}

// Skipped segments are named with their reasons in one Warning, and the state is still used.
func TestSkippedSegmentsLogged(t *testing.T) {
	cfg := bootTestConfig("")
	client := bootPopulatedRedis(t, cfg, 0)
	delete(client.hashes[bootTestKeys(t, cfg).Meta()], bootTestKeys(t, cfg).SegmentCursorField("s2"))
	logger := &bootTestLogger{}

	decision, err := resolveRedisBoot(cfg, logger, func(*pconf.Main) (bootReadClient, error) { return client, nil })

	require.NoError(t, err)
	assert.Equal(t, BootSourceRedis, decision.Source)
	assert.Equal(t, []string{"s2"}, decision.State.SkippedSegments)
	assert.True(t, logger.has("warning", "s2 (referenced by a flag or rule-based segment but has no cursor in meta)"))
}

// bootAdminGet retries a GET until the admin listener answers or 2s pass, and returns the status and decoded JSON body.
func bootAdminGet(t *testing.T, url string) (int, map[string]interface{}) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := client.Get(url)
		if err != nil {
			if time.Now().After(deadline) {
				t.Fatalf("GET %s: %v", url, err)
			}
			continue
		}
		defer func() { _ = resp.Body.Close() }()
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
}

func TestBootAdminPhaseIsServed(t *testing.T) {
	cfg := bootTestConfig("")
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = freeTCPPort(t)
	b, err := startBootAdmin(cfg, &bootTestLogger{})
	require.NoError(t, err)
	t.Cleanup(b.stop)
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.Admin.Port)

	status, body := bootAdminGet(t, base+"/health/application")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, body["healthy"])
	redis, ok := body["redis"].(map[string]interface{})
	require.True(t, ok, "redis section missing in %v", body)
	assert.Equal(t, bootPhaseReading, redis["bootPhase"])

	b.setPhase(bootPhaseInitialSync)
	status, body = bootAdminGet(t, base+"/health/application")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, body["healthy"])
	redis, ok = body["redis"].(map[string]interface{})
	require.True(t, ok, "redis section missing in %v", body)
	assert.Equal(t, bootPhaseInitialSync, redis["bootPhase"])

	status, _ = bootAdminGet(t, base+"/other")
	assert.Equal(t, http.StatusServiceUnavailable, status)
}

func TestStartBootAdminInvalidTLSIsExitTLSError(t *testing.T) {
	cfg := bootTestConfig("")
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = freeTCPPort(t)
	dir := t.TempDir()
	cfg.Admin.TLS = conf.TLS{
		Enabled:       true,
		MinTLSVersion: "1.3",
		CertChainFN:   filepath.Join(dir, "missing-cert.pem"),
		PrivateKeyFN:  filepath.Join(dir, "missing-key.pem"),
	}

	b, err := startBootAdmin(cfg, &bootTestLogger{})
	requireBootExitCode(t, err, common.ExitTLSError)
	assert.Nil(t, b)

	l, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port))
	require.NoError(t, listenErr, "admin port must not stay bound after a TLS failure")
	require.NoError(t, l.Close())
}

func TestBootAdminStopIsIdempotentAndNilSafe(t *testing.T) {
	var nilAdmin *bootAdmin
	assert.NotPanics(t, nilAdmin.stop)

	cfg := bootTestConfig("")
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = freeTCPPort(t)
	b, err := startBootAdmin(cfg, &bootTestLogger{})
	require.NoError(t, err)
	b.stop()
	select {
	case <-b.done:
	default:
		t.Fatal("serve goroutine still running after stop")
	}
	assert.NotPanics(t, b.stop)

	l, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port))
	require.NoError(t, listenErr, "admin port must be released by stop")
	require.NoError(t, l.Close())
}

func TestStartBootAdminPortInUseIsExitAdminError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }() // best-effort test cleanup

	cfg := bootTestConfig("")
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = int64(l.Addr().(*net.TCPAddr).Port)
	b, startErr := startBootAdmin(cfg, &bootTestLogger{})
	requireBootExitCode(t, startErr, common.ExitAdminError)
	assert.Nil(t, b)
}

// skippedSummary names at most maxLoggedSkipped segments with their reasons and reports the rest as a count.
func TestSkippedSummaryCapsAtTwenty(t *testing.T) {
	state := redisstate.BootState{SkipReasons: map[string]string{}}
	for i := 1; i <= 21; i++ {
		name := fmt.Sprintf("seg-%02d", i)
		state.SkippedSegments = append(state.SkippedSegments, name)
		state.SkipReasons[name] = "torn"
	}

	summary := skippedSummary(state)

	parts := strings.Split(summary, "; ")
	require.Len(t, parts, maxLoggedSkipped+1)
	assert.Equal(t, "seg-01 (torn)", parts[0])
	assert.Equal(t, "seg-20 (torn)", parts[maxLoggedSkipped-1])
	assert.Equal(t, "and 1 more", parts[maxLoggedSkipped])
	assert.NotContains(t, summary, "seg-21")
	assert.True(t, strings.HasSuffix(summary, "; and 1 more"))
}

// resolveRedisBoot is a test helper for the nil-sink path of resolveRedisBootInto.
func resolveRedisBoot(cfg *pconf.Main, logger logging.LoggerInterface, open bootClientOpener) (BootDecision, error) {
	return resolveRedisBootInto(cfg, logger, open, nil)
}

// decideFromState: a budget-exhausted corrupt state logs an error and falls back.
func TestDecideFromStateBudgetExhaustedLogsError(t *testing.T) {
	cfg := bootTestConfig("")
	logger := &bootTestLogger{}
	state := redisstate.BootState{Status: redisstate.BootCorrupt, BudgetExhausted: true, Reason: "budget"}

	decision, err := decideFromState(cfg, logger, state)

	require.NoError(t, err)
	assert.NotEqual(t, BootSourceRedis, decision.Source)
	assert.Equal(t, BootSourceCold, decision.Source)
	assert.Equal(t, "redis restore time limit reached", decision.Reason)
	assert.True(t, logger.has("error", "restore time limit reached"), "expected an error log")
}

// decideFromState: an unknown status falls back with a warning.
func TestDecideFromStateUnknownStatusFallsBack(t *testing.T) {
	cfg := bootTestConfig("/tmp/snapshot.json")
	logger := &bootTestLogger{}
	state := redisstate.BootState{Status: redisstate.BootStatus(99)}

	decision, err := decideFromState(cfg, logger, state)

	require.NoError(t, err)
	assert.NotEqual(t, BootSourceRedis, decision.Source)
	assert.Equal(t, BootSourceSnapshotFile, decision.Source)
	assert.Equal(t, "unexpected redis state status", decision.Reason)
	assert.True(t, logger.has("warning", "unexpected redis state status"), "expected a warning log")
}

// decideFromState: a corrupt state without budget exhaustion logs a warning, not an error.
func TestDecideFromStateCorruptWithoutBudgetLogsWarning(t *testing.T) {
	cfg := bootTestConfig("")
	logger := &bootTestLogger{}
	state := redisstate.BootState{Status: redisstate.BootCorrupt, Reason: "bad crc"}

	decision, err := decideFromState(cfg, logger, state)

	require.NoError(t, err)
	assert.NotEqual(t, BootSourceRedis, decision.Source)
	assert.Equal(t, "corrupt redis state", decision.Reason)
	assert.True(t, logger.has("warning", "redis state is corrupt"), "expected a warning log")
	assert.False(t, logger.has("error", ""), "no error log expected")
}
