package proxy

import (
	"fmt"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	commonredis "github.com/splitio/go-split-commons/v10/storage/redis"
)

// redisStateStopTimeout bounds how long shutdown waits for the Redis writer to stop.
const redisStateStopTimeout = 10 * time.Second

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

// setupRedisState builds the Redis writer that mirrors the proxy state. It returns (nil, nil) when the
// feature is disabled. The writer is started (after an optional fresh-startup reset) before it is returned.
func setupRedisState(cfg *pconf.Main, logger logging.LoggerInterface, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segments *storage.ProxySegmentStorageImpl) (*redisstate.Writer, error) {
	if !cfg.Storage.Redis.Enabled {
		return nil, nil
	}

	redisCfg := cfg.Storage.Redis
	opts, err := common.ParseRedisOptions(&redisCfg.Connection)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error parsing redis state config: %w", err), common.ExitInvalidConfiguration)
	}

	client, err := commonredis.NewRedisClient(opts, logger)
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

	fingerprint := util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
	writer, err := redisstate.NewWriter(client, keys, fingerprint, options, logger)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error creating redis state writer: %w", err), common.ExitInvalidConfiguration)
	}

	if cfg.Initialization.ForceFreshStartup {
		if err := writer.Reset(); err != nil {
			return nil, common.NewInitError(fmt.Errorf("error resetting redis state: %w", err), common.ExitRedisInitializationFailed)
		}
	}

	splits.WithRedisWriter(writer)
	rbs.WithRedisWriter(writer)
	segments.WithRedisWriter(writer)

	writer.Start(storage.NewRedisStateSource(splits, rbs, segments))
	return writer, nil
}
