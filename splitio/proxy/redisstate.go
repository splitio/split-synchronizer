package proxy

import (
	"fmt"
	"math"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	commonredis "github.com/splitio/go-split-commons/v10/storage/redis"
)

// stopWaitBound is the hard bound shutdown waits for the Redis writer to stop (D26 (a)). It is the drain bound
// plus one worst-case Redis call (dial, read and write timeouts for every attempt, the timeouts are in seconds)
// plus a fixed margin, so a single hung Eval cannot exceed it.
func stopWaitBound(state pconf.RedisState, conn conf.Redis) time.Duration {
	drain := time.Duration(state.StopTimeoutMs) * time.Millisecond
	perCall := time.Duration(conn.DialTimeout+conn.ReadTimeout+conn.WriteTimeout) * time.Second
	attempts := time.Duration(conn.MaxRetries + 1)
	return drain + perCall*attempts + 5*time.Second
}

// stopRedisState stops the Redis writer on shutdown. Stop runs in its own goroutine so that a writer stuck on Redis
// cannot block the process exit. It returns false when Stop did not return within timeout.
func stopRedisState(writer *redisstate.Writer, logger logging.LoggerInterface, timeout time.Duration) bool {
	if writer == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		writer.Stop()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		logger.Warning(fmt.Sprintf("redis state writer did not stop within %s, continuing shutdown", timeout))
		return false
	}
}

// Upper bounds for the remaining settings. Chunk size, purge batch and stop timeout use redisstate.Max* so they
// match the limits NewWriter enforces.
const (
	redisStateMaxPendingChanges = 10000000
	redisStateMaxWriteAttempts  = 100
	redisStateMaxBackoffMs      = 3600000
	redisStateMaxConnTimeoutSec = 300
	redisStateMaxConnRetries    = 10
)

// validateRedisStateSettings rejects values the writer can not run with. It runs before any Redis connection
// attempt, so invalid configuration never reaches the network.
func validateRedisStateSettings(cfg *pconf.RedisState) error {
	invalid := func(format string, args ...interface{}) error {
		return common.NewInitError(fmt.Errorf("invalid redis state configuration: "+format, args...), common.ExitInvalidConfiguration)
	}
	switch {
	case cfg.ChunkSize <= 0 || cfg.ChunkSize > redisstate.MaxChunkSize:
		return invalid("chunkSize must be 1..%d, got %d", redisstate.MaxChunkSize, cfg.ChunkSize)
	case cfg.PurgeBatch <= 0 || cfg.PurgeBatch > redisstate.MaxPurgeBatch:
		return invalid("purgeBatch must be 1..%d, got %d", redisstate.MaxPurgeBatch, cfg.PurgeBatch)
	case cfg.StopTimeoutMs <= 0 || cfg.StopTimeoutMs > redisstate.MaxStopTimeoutMs:
		return invalid("stopTimeoutMs must be 1..%d, got %d", redisstate.MaxStopTimeoutMs, cfg.StopTimeoutMs)
	case cfg.MaxPendingChanges <= 0:
		return invalid("maxPendingChanges must be positive, got %d", cfg.MaxPendingChanges)
	case cfg.MaxPendingChanges > redisStateMaxPendingChanges:
		return invalid("maxPendingChanges must be at most %d, got %d", redisStateMaxPendingChanges, cfg.MaxPendingChanges)
	case cfg.WriteMaxAttempts <= 0 || cfg.WriteMaxAttempts > redisStateMaxWriteAttempts:
		return invalid("writeMaxAttempts must be 1..%d, got %d", redisStateMaxWriteAttempts, cfg.WriteMaxAttempts)
	case cfg.WriteBackoffMaxMs <= 0 || cfg.WriteBackoffMaxMs > redisStateMaxBackoffMs:
		// Above this bound the ms to Duration conversion is at risk of overflowing int64 nanoseconds.
		return invalid("writeBackoffMaxMs must be 1..%d, got %d", redisStateMaxBackoffMs, cfg.WriteBackoffMaxMs)
	case cfg.Connection.DialTimeout < 0 || cfg.Connection.DialTimeout > redisStateMaxConnTimeoutSec:
		return invalid("dialTimeout must be 0..%d seconds, got %d", redisStateMaxConnTimeoutSec, cfg.Connection.DialTimeout)
	case cfg.Connection.ReadTimeout < 0 || cfg.Connection.ReadTimeout > redisStateMaxConnTimeoutSec:
		return invalid("readTimeout must be 0..%d seconds, got %d", redisStateMaxConnTimeoutSec, cfg.Connection.ReadTimeout)
	case cfg.Connection.WriteTimeout < 0 || cfg.Connection.WriteTimeout > redisStateMaxConnTimeoutSec:
		return invalid("writeTimeout must be 0..%d seconds, got %d", redisStateMaxConnTimeoutSec, cfg.Connection.WriteTimeout)
	case cfg.Connection.MaxRetries < 0 || cfg.Connection.MaxRetries > redisStateMaxConnRetries:
		return invalid("maxRetries must be 0..%d, got %d", redisStateMaxConnRetries, cfg.Connection.MaxRetries)
	case cfg.RetentionHours <= 0:
		return invalid("retentionHours must be positive, got %d", cfg.RetentionHours)
	case cfg.RetentionHours > math.MaxInt64/int64(time.Hour):
		// The writer stores retention as a time.Duration (nanoseconds), so this is the bound that prevents overflow.
		return invalid("retentionHours must be at most %d, got %d", math.MaxInt64/int64(time.Hour), cfg.RetentionHours)
	}
	return nil
}

var newRedisStateClient = commonredis.NewRedisClient

// setupRedisState builds the Redis writer that mirrors the proxy state. It returns (nil, nil) when the
// feature is disabled. The writer is started (after an optional fresh-startup reset) before it is returned.
func setupRedisState(cfg *pconf.Main, logger logging.LoggerInterface, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segments *storage.ProxySegmentStorageImpl) (*redisstate.Writer, error) {
	if !cfg.Storage.Redis.Enabled {
		return nil, nil
	}

	redisCfg := cfg.Storage.Redis
	if err := validateRedisStateSettings(&redisCfg); err != nil {
		return nil, err
	}

	opts, err := common.ParseRedisOptions(&redisCfg.Connection)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error parsing redis state config: %w", err), common.ExitInvalidConfiguration)
	}

	// The go-toolkit PrefixedRedisClient has no Close, so on a later setup failure this client is not closed.
	// Accepted and documented (plan D29 C-6, OI18): the process exit releases it.
	client, err := newRedisStateClient(opts, logger)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error instantiating redis state client: %w", err), common.ExitRedisInitializationFailed)
	}

	keys, err := redisstate.NewKeys(client.Prefix(), cfg.Apikey, redisCfg.Namespace)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}

	options := redisstate.DefaultOptions()
	options.Retention = time.Duration(redisCfg.RetentionHours) * time.Hour
	options.MaxAttempts = int(redisCfg.WriteMaxAttempts)
	options.BackoffMaxWait = time.Duration(redisCfg.WriteBackoffMaxMs) * time.Millisecond
	options.MaxPendingChanges = int(redisCfg.MaxPendingChanges)
	options.ChunkSize = int(redisCfg.ChunkSize)
	options.PurgeBatch = int(redisCfg.PurgeBatch)
	options.StopTimeoutMs = redisCfg.StopTimeoutMs

	fingerprint := util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
	writer, err := redisstate.NewWriter(client, keys, fingerprint, options, logger)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error creating redis state writer: %w", err), common.ExitInvalidConfiguration)
	}
	logger.Info(fmt.Sprintf("redis state writer shutdown wait bound %s (drain %d ms, per-call timeouts %d+%d+%d s, retries %d)",
		stopWaitBound(redisCfg, redisCfg.Connection), redisCfg.StopTimeoutMs, redisCfg.Connection.DialTimeout,
		redisCfg.Connection.ReadTimeout, redisCfg.Connection.WriteTimeout, redisCfg.Connection.MaxRetries))

	if cfg.Initialization.ForceFreshStartup {
		if err := writer.Reset(); err != nil {
			return nil, common.NewInitError(fmt.Errorf("error resetting redis state: %w", err), common.ExitRedisInitializationFailed)
		}
	}

	splits.WithRedisWriter(writer)
	rbs.WithRedisWriter(writer)
	segments.WithRedisWriter(writer)

	if err := writer.Start(storage.NewRedisStateSource(splits, rbs, segments)); err != nil {
		return nil, err
	}
	return writer, nil
}
