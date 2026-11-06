package proxy

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	commonredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
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
