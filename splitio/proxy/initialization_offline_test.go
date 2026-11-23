package proxy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	goredis "github.com/redis/go-redis/v9"
	"github.com/splitio/go-split-commons/v10/dtos"
	commonsflagsets "github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

func writeTestSnapshot(t *testing.T, meta snapshot.Metadata, payload []byte) string {
	t.Helper()
	snap, err := snapshot.New(meta, payload)
	assert.Nil(t, err)
	raw, err := snap.Encode()
	assert.Nil(t, err)
	path := filepath.Join(t.TempDir(), "proxy.snapshot")
	assert.Nil(t, os.WriteFile(path, raw, 0644))
	return path
}

func assertInvalidConfiguration(t *testing.T, err error) {
	t.Helper()
	var initErr *common.InitializationError
	if assert.True(t, errors.As(err, &initErr), "expected an initialization error, got %v", err) {
		assert.Equal(t, common.ExitInvalidConfiguration, initErr.ExitCode())
	}
}

func TestLoadOfflineSnapshot(t *testing.T) {
	payload := []byte("bolt-bytes")
	validMeta := snapshot.Metadata{
		Version:         1,
		Storage:         snapshot.StorageBoltDB,
		Hash:            "unrelated-hash",
		Checksum:        snapshot.PayloadChecksum(payload),
		FlagSpecVersion: "1.3",
	}

	t.Run("valid snapshot loads regardless of hash", func(t *testing.T) {
		cfg := &pconf.Main{FlagSpecVersion: "1.2"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, validMeta, payload)
		snap, data, err := loadOfflineSnapshot(cfg)
		assert.Nil(t, err)
		assert.Equal(t, payload, data)
		assert.Equal(t, "1.3", snap.Meta().FlagSpecVersion)
	})

	t.Run("api key is rejected", func(t *testing.T) {
		cfg := &pconf.Main{Apikey: "someKey", FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, validMeta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("missing snapshot path is rejected", func(t *testing.T) {
		_, _, err := loadOfflineSnapshot(&pconf.Main{FlagSpecVersion: "1.3"})
		assertInvalidConfiguration(t, err)
	})

	t.Run("unreadable snapshot file is rejected", func(t *testing.T) {
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = filepath.Join(t.TempDir(), "missing.snapshot")
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("snapshot without checksum is rejected", func(t *testing.T) {
		meta := validMeta
		meta.Checksum = ""
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, meta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("unsupported configured spec is rejected", func(t *testing.T) {
		cfg := &pconf.Main{FlagSpecVersion: "9.9"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, validMeta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("checksum mismatch is rejected", func(t *testing.T) {
		meta := validMeta
		meta.Checksum = snapshot.PayloadChecksum([]byte("other-bytes"))
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, meta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})
}

// Treatment overrides (FME-19233) rely on offline loading leaving the Bolt data untouched,
// so a re-exported snapshot keeps the checksum of the file that was loaded.
func TestOfflineLoadPreservesExportChecksum(t *testing.T) {
	logger := logging.NewLogger(nil)
	src, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "src.db"), nil)
	assert.Nil(t, err)
	storage.NewProxySplitStorage(src, logger, commonsflagsets.NewFlagSetFilter(nil), false).Update(
		[]dtos.SplitDTO{{Name: "s1", Status: "ACTIVE", ChangeNumber: 10}, {Name: "s2", Status: "ACTIVE", ChangeNumber: 12}},
		nil,
		12,
	)
	exported, err := src.GetRawSnapshot()
	assert.Nil(t, err)

	path, err := snapshot.WritePayloadToTmpFile(exported)
	assert.Nil(t, err)
	defer os.Remove(path)
	db, err := persistent.NewBoltWrapper(path, nil)
	assert.Nil(t, err)
	storage.NewProxySplitStorage(db, logger, commonsflagsets.NewFlagSetFilter(nil), true)
	storage.NewProxyRuleBasedSegmentsStorage(db, logger, true)
	storage.NewProxySegmentStorage(db, logger, true)

	reexported, err := db.GetRawSnapshot()
	assert.Nil(t, err)
	assert.Equal(t, snapshot.PayloadChecksum(exported), snapshot.PayloadChecksum(reexported))
}

func TestStartRejectsOverridesFileOutsideOfflineMode(t *testing.T) {
	logger := logging.NewLogger(nil)
	const wantMsg = "treatment overrides require offline mode in this version"

	t.Run("fails without offline mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "overrides.yaml")
		assert.Nil(t, os.WriteFile(path, []byte(`new_checkout: "off"`), 0644))
		cfg := &pconf.Main{Apikey: "someKey", FlagSpecVersion: "1.3"}
		cfg.Initialization.OverridesFile = path

		err := Start(logger, cfg)
		assertInvalidConfiguration(t, err)
		assert.Contains(t, err.Error(), wantMsg)
	})

	t.Run("fails without offline mode even when the file is empty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "overrides.yaml")
		assert.Nil(t, os.WriteFile(path, nil, 0644))
		cfg := &pconf.Main{Apikey: "someKey", FlagSpecVersion: "1.3"}
		cfg.Initialization.OverridesFile = path

		err := Start(logger, cfg)
		assertInvalidConfiguration(t, err)
		assert.Contains(t, err.Error(), wantMsg)
	})

	t.Run("unset option does not trigger the check", func(t *testing.T) {
		err := Start(logger, &pconf.Main{FlagSpecVersion: "1.3"})
		if assert.NotNil(t, err) {
			assert.NotContains(t, err.Error(), wantMsg)
		}
	})
}

func TestOfflineWarnsWhenFlagSetsFilterIsConfigured(t *testing.T) {
	logger := &mockLogger{}
	warnOfflineFlagSetsFilter(logger, nil)
	warnOfflineFlagSetsFilter(logger, []string{})
	if len(logger.warnings) != 0 {
		t.Errorf("no warning expected without a filter, got %v", logger.warnings)
	}

	warnOfflineFlagSetsFilter(logger, []string{"set_a", "set_b"})
	if len(logger.warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", logger.warnings)
	}
	if !strings.Contains(logger.warnings[0], "set_a,set_b") || !strings.Contains(logger.warnings[0], "ignored") {
		t.Errorf("warning should name the filter and say it is ignored: %q", logger.warnings[0])
	}
}

// The offline health endpoints dereference these monitors, so they must exist and report healthy.
// seedOfflineRedis writes one flag under namespace through the writer and returns the fingerprint an offline config
// must name to read that state.
func seedOfflineRedis(t *testing.T, namespace string) string {
	t.Helper()
	seed := redisStateTestConfig(t)
	seed.Storage.Redis.Namespace = namespace
	startSeedRedisFlags(t, seed)
	return util.ConfigFingerprint(seed.Apikey, seed.FlagSpecVersion, seed.FlagSetsFilter)
}

// offlineRedisCfg is an offline config that reads the redis state of namespace. The snapshot path does not exist, so
// the boot falls through to redis.
func offlineRedisCfg(t *testing.T, namespace, fingerprint string) *pconf.Main {
	t.Helper()
	cfg := redisStateTestConfig(t)
	cfg.Apikey = ""
	cfg.Offline = true
	cfg.Initialization.Snapshot = filepath.Join(t.TempDir(), "absent.snapshot")
	cfg.Storage.Redis.Namespace = namespace
	cfg.Storage.Redis.OfflineFingerprint = fingerprint
	return cfg
}

// writeOfflineBoltSnapshot writes a snapshot of an empty Bolt database with the payload checksum offline mode requires.
func writeOfflineBoltSnapshot(t *testing.T, flagSpec string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "payload.db")
	db, err := persistent.NewBoltWrapper(dbPath, nil)
	if err != nil {
		t.Fatalf("bolt payload: %v", err)
	}
	if err := db.Discard(); err != nil {
		t.Fatalf("close bolt payload: %v", err)
	}
	payload, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read bolt payload: %v", err)
	}
	return writeTestSnapshot(t, snapshot.Metadata{
		Version:         1,
		Storage:         snapshot.StorageBoltDB,
		Checksum:        snapshot.PayloadChecksum(payload),
		FlagSpecVersion: flagSpec,
	}, payload)
}

func TestOfflineRedisBootReadOnly(t *testing.T) {
	logger := logging.NewLogger(nil)

	t.Run("snapshot absent and matching redis state boots from redis", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlinevalid")
		boot, err := bootOffline(logger, offlineRedisCfg(t, "offlinevalid", fp))
		if err != nil {
			t.Fatalf("expected a boot from redis, got %v", err)
		}
		t.Cleanup(func() { _ = boot.dbInstance.Discard() })
		assert.Equal(t, BootSourceRedis, boot.source)
		assert.NotNil(t, boot.splitStorage.Split("startFlag"))
	})

	t.Run("snapshot absent and foreign fingerprint is not used", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlineforeign")
		_, err := bootOffline(logger, offlineRedisCfg(t, "offlineforeign", fp+"1"))
		assertInvalidConfiguration(t, err)
	})

	t.Run("missing namespace is rejected", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlinenons")
		_, err := bootOffline(logger, offlineRedisCfg(t, "", fp))
		assertInvalidConfiguration(t, err)
	})

	t.Run("missing offline fingerprint is rejected", func(t *testing.T) {
		seedOfflineRedis(t, "offlinenofp")
		_, err := bootOffline(logger, offlineRedisCfg(t, "offlinenofp", ""))
		assertInvalidConfiguration(t, err)
	})

	t.Run("no snapshot and no redis state is rejected", func(t *testing.T) {
		_, err := bootOffline(logger, offlineRedisCfg(t, "offlineempty", "1"))
		assertInvalidConfiguration(t, err)
	})

	t.Run("usable snapshot is served and redis is not read", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlinesnap")
		opens := countBootReads(t)
		cfg := offlineRedisCfg(t, "offlinesnap", fp)
		cfg.Initialization.Snapshot = writeOfflineBoltSnapshot(t, cfg.FlagSpecVersion)
		boot, err := bootOffline(logger, cfg)
		if err != nil {
			t.Fatalf("expected the snapshot to boot, got %v", err)
		}
		t.Cleanup(func() { _ = boot.dbInstance.Discard() })
		assert.Equal(t, BootSourceSnapshotFile, boot.source)
		assert.Equal(t, int32(0), opens.Load())
	})
}

// offlineReadCommands are the only commands the offline boot read may issue.
var offlineReadCommands = map[string]bool{"HGetAll": true, "ZScan": true, "ZCard": true, "HGet": true, "Prefix": true, "Close": true}

// offlineBootRecorder collects the commands issued through the boot read clients of one test.
type offlineBootRecorder struct {
	mtx   sync.Mutex
	calls []string
}

func (r *offlineBootRecorder) note(name string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.calls = append(r.calls, name)
}

func (r *offlineBootRecorder) reset() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.calls = nil
}

func (r *offlineBootRecorder) recorded() []string {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return append([]string(nil), r.calls...)
}

// offlineRecordingClient forwards the boot read to the real client and records each command.
type offlineRecordingClient struct {
	bootReadClient
	rec *offlineBootRecorder
}

func (c *offlineRecordingClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	c.rec.note("HGetAll")
	return c.bootReadClient.HGetAll(ctx, key)
}

func (c *offlineRecordingClient) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	c.rec.note("ZScan")
	return c.bootReadClient.ZScan(ctx, key, cursor, count)
}

func (c *offlineRecordingClient) ZCard(ctx context.Context, key string) (int64, error) {
	c.rec.note("ZCard")
	return c.bootReadClient.ZCard(ctx, key)
}

func (c *offlineRecordingClient) HGet(ctx context.Context, key, field string) (string, bool, error) {
	c.rec.note("HGet")
	return c.bootReadClient.HGet(ctx, key, field)
}

// recordOfflineBootReads wraps the boot read client seam so every offline boot read of the test is recorded.
func recordOfflineBootReads(t *testing.T) *offlineBootRecorder {
	t.Helper()
	rec := &offlineBootRecorder{}
	orig := newBootReadClient
	newBootReadClient = func(c *pconf.Main) (bootReadClient, error) {
		client, err := orig(c)
		if err != nil {
			return nil, err
		}
		return &offlineRecordingClient{bootReadClient: client, rec: rec}, nil
	}
	t.Cleanup(func() { newBootReadClient = orig })
	return rec
}

// assertOfflineBootReadOnly checks that the offline boot issued only read commands and left no lease key in Redis.
func assertOfflineBootReadOnly(t *testing.T, rec *offlineBootRecorder, cfg *pconf.Main) {
	t.Helper()
	for _, call := range rec.recorded() {
		if !offlineReadCommands[call] {
			t.Errorf("offline boot issued %s, only read commands are allowed", call)
		}
	}
	raw := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer func() { _ = raw.Close() }()
	keys, err := redisstate.NewKeys(cfg.Storage.Redis.Connection.Prefix, "", cfg.Storage.Redis.Namespace)
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	n, err := raw.Exists(context.Background(), keys.Lease()).Result()
	if err != nil {
		t.Fatalf("lease key lookup: %v", err)
	}
	assert.Equal(t, int64(0), n, "offline boot must not leave a lease key")
}

func TestOfflineNeverEvalsLease(t *testing.T) {
	logger := logging.NewLogger(nil)
	rec := recordOfflineBootReads(t)

	t.Run("valid redis state", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlineeval-valid")
		cfg := offlineRedisCfg(t, "offlineeval-valid", fp)
		rec.reset()
		boot, err := bootOffline(logger, cfg)
		if err != nil {
			t.Fatalf("expected a boot from redis, got %v", err)
		}
		t.Cleanup(func() { _ = boot.dbInstance.Discard() })
		assert.NotEmpty(t, rec.recorded(), "the redis boot must read")
		assertOfflineBootReadOnly(t, rec, cfg)
	})

	t.Run("foreign redis state", func(t *testing.T) {
		fp := seedOfflineRedis(t, "offlineeval-foreign")
		cfg := offlineRedisCfg(t, "offlineeval-foreign", fp+"1")
		rec.reset()
		_, err := bootOffline(logger, cfg)
		assertInvalidConfiguration(t, err)
		assertOfflineBootReadOnly(t, rec, cfg)
	})

	t.Run("no redis state", func(t *testing.T) {
		cfg := offlineRedisCfg(t, "offlineeval-absent", "1")
		rec.reset()
		_, err := bootOffline(logger, cfg)
		assertInvalidConfiguration(t, err)
		assertOfflineBootReadOnly(t, rec, cfg)
	})
}

func TestOfflineRedisRejectsForceFresh(t *testing.T) {
	fp := seedOfflineRedis(t, "offlineforcefresh")
	cfg := offlineRedisCfg(t, "offlineforcefresh", fp)
	cfg.Initialization.ForceFreshStartup = true
	_, err := bootOffline(logging.NewLogger(nil), cfg)
	assertInvalidConfiguration(t, err)
	assert.Contains(t, err.Error(), "does not apply in offline mode")
}

// TestOfflineRedisRejectsForceFreshWithUsableSnapshot covers PR review comment 2: the flag is rejected before the
// snapshot is tried, so a readable snapshot file does not let the invalid configuration start.
func TestOfflineRedisRejectsForceFreshWithUsableSnapshot(t *testing.T) {
	fp := seedOfflineRedis(t, "offlineforcefreshsnap")
	cfg := offlineRedisCfg(t, "offlineforcefreshsnap", fp)
	cfg.Initialization.Snapshot = writeOfflineBoltSnapshot(t, cfg.FlagSpecVersion)
	cfg.Initialization.ForceFreshStartup = true
	opens := countBootReads(t)
	_, err := bootOffline(logging.NewLogger(nil), cfg)
	assertInvalidConfiguration(t, err)
	assert.Contains(t, err.Error(), "does not apply in offline mode")
	assert.Equal(t, int32(0), opens.Load())
}

func TestOfflineRedisRejectsCluster(t *testing.T) {
	cfg := redisStateTestConfig(t)
	cfg.Apikey = ""
	cfg.Offline = true
	cfg.Storage.Redis.Namespace = "offlinecluster"
	cfg.Storage.Redis.OfflineFingerprint = "1"
	cfg.Storage.Redis.Connection.ClusterMode = true
	assertInvalidConfiguration(t, Start(logging.NewLogger(nil), cfg))
}

func TestOfflineStaticMonitorsReportHealthy(t *testing.T) {
	app := newStaticAppMonitor()
	services := newStaticServicesMonitor()

	app.Start()
	services.Start()
	app.NotifyEvent(0)
	app.Reset(0, 0)

	appStatus := app.GetHealthStatus()
	assert.True(t, appStatus.Healthy)
	assert.NotNil(t, appStatus.HealthySince)
	assert.NotNil(t, appStatus.Items)
	assert.Empty(t, appStatus.Items)

	svcStatus := services.GetHealthStatus()
	assert.Equal(t, "healthy", svcStatus.Status)
	if assert.Len(t, svcStatus.Items, 1) {
		assert.True(t, svcStatus.Items[0].Healthy)
		assert.Equal(t, "cloud checks disabled", svcStatus.Items[0].Message)
	}

	app.Stop()
	services.Stop()
}

func TestOfflineWarnsWhenSnapshotReferencesLargeSegments(t *testing.T) {
	logger := &mockLogger{}
	warnOfflineLargeSegments(logger, nil)
	warnOfflineLargeSegments(logger, set.NewSet())
	if len(logger.warnings) != 0 {
		t.Fatalf("no warning expected without large segments, got %v", logger.warnings)
	}

	warnOfflineLargeSegments(logger, set.NewSet("seg_a", "seg_b"))
	if len(logger.warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", logger.warnings)
	}
	msg := logger.warnings[0]
	if !strings.Contains(msg, "seg_a") || !strings.Contains(msg, "seg_b") {
		t.Errorf("warning should name the large segments: %q", msg)
	}
	if !strings.Contains(msg, "default treatment") {
		t.Errorf("warning should say flags return the default treatment: %q", msg)
	}
}
