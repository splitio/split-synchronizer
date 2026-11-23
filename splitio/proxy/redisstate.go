package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/splitio/go-toolkit/v5/backoff"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	commonconf "github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/dtos"
	commonredis "github.com/splitio/go-split-commons/v10/storage/redis"
)

// stopWaitBound is the hard bound shutdown waits for the Redis writer to stop. It is the drain bound
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
func stopRedisState(writer interface{ Stop() }, logger logging.LoggerInterface, timeout time.Duration) bool {
	if writer == nil {
		return true
	}
	logger.Info("stopping redis writer, waiting for the queue to drain")
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
	redisStateMaxPoolSize       = 10000
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
	case cfg.Connection.PoolSize <= 0 || cfg.Connection.PoolSize > redisStateMaxPoolSize:
		return invalid("poolSize must be 1..%d, got %d", redisStateMaxPoolSize, cfg.Connection.PoolSize)
	case cfg.Connection.Db < 0:
		return invalid("db must be 0 or greater, got %d", cfg.Connection.Db)
	case cfg.RetentionHours <= 0:
		return invalid("retentionHours must be positive, got %d", cfg.RetentionHours)
	case cfg.RetentionHours > math.MaxInt64/int64(time.Hour):
		// The writer stores retention as a time.Duration (nanoseconds), so this is the bound that prevents overflow.
		return invalid("retentionHours must be at most %d, got %d", math.MaxInt64/int64(time.Hour), cfg.RetentionHours)
	}
	return nil
}

var newRedisStateClient = commonredis.NewRedisClient

// redisStateRetryDelay shapes the wait before each reconnect attempt. Tests replace it to avoid real waits.
var redisStateRetryDelay = func(d time.Duration) time.Duration { return d }

// setupRedisState builds the Redis writer that mirrors the proxy state. It returns (nil, nil) when the feature is
// disabled. Configuration errors and non-connection client errors are returned synchronously. When Redis is not
// reachable the proxy keeps running: the returned supervisor retries in the background and attaches the writer once
// Redis answers. A nil gate gives the writer the local state as it is; a closed gate hides it until the first sync completes.
func setupRedisState(cfg *pconf.Main, logger logging.LoggerInterface, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segments *storage.ProxySegmentStorageImpl, gate *sourceGate) (*redisStateSupervisor, error) {
	if !cfg.Storage.Redis.Enabled {
		return nil, nil
	}

	redisCfg := cfg.Storage.Redis
	if err := validateRedisStateSettings(&redisCfg); err != nil {
		return nil, err
	}
	// The namespace is checked here, before any connection, so a bad value never reaches Redis. The prefix is
	// only known once the client exists and is validated by the NewKeys call below.
	if _, err := redisstate.NewKeys("", cfg.Apikey, redisCfg.Namespace); err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}

	opts, err := common.ParseRedisStateOptions(&redisCfg.Connection)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error parsing redis state config: %w", err), common.ExitInvalidConfiguration)
	}

	s := &redisStateSupervisor{
		cfg:      cfg,
		logger:   logger,
		splits:   splits,
		rbs:      rbs,
		segments: segments,
		gate:     gate,
		opts:     opts,
		delay:    redisStateRetryDelay,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	// The go-toolkit PrefixedRedisClient has no Close, so on a later setup failure this client is not closed; process exit releases it.
	client, err := newRedisStateClient(opts, logger)
	if err != nil {
		if !isRedisUnavailable(err) {
			return nil, common.NewInitError(fmt.Errorf("error instantiating redis state client: %w", err), common.ExitRedisInitializationFailed)
		}
		logger.Error(fmt.Sprintf("redis state: Redis is not reachable, the proxy keeps running and retries in the background: %v", err))
		s.setLastErr("redis state writer waiting for Redis")
		go s.retryLoop()
		return s, nil
	}

	writer, retry, err := s.attach(client)
	if err != nil {
		if !retry {
			return nil, err
		}
		// Redis dropped between the connect and the reset or start: handle it like a dial failure.
		s.detach()
		logger.Error(fmt.Sprintf("redis state: Redis went away while starting the writer, the proxy keeps running and retries in the background: %v", err))
		s.setLastErr("redis state writer waiting for Redis")
		go s.retryLoop()
		return s, nil
	}
	close(s.done) // no retry goroutine: Stop has nothing to wait for
	s.writer.Store(writer)
	return s, nil
}

// errRedisStateStopped is returned by attach when shutdown began before the writer was wired.
var errRedisStateStopped = errors.New("redis state supervisor stopped")

// isRedisUnavailable reports whether err means Redis could not be reached, as opposed to a configuration or protocol error.
func isRedisUnavailable(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), redisSentinelsUnreachable)
}

// redisSentinelsUnreachable is the message go-redis (v9.7.3, sentinel.go) returns when no sentinel answers. It is a
// plain error with no exported value, so the message is the only thing to match on.
const redisSentinelsUnreachable = "all sentinels specified in configuration are unreachable"

// redisStateSupervisor owns the Redis writer for the lifetime of the proxy. When Redis is unreachable at startup it
// keeps the proxy running and attaches the writer from a background goroutine once Redis answers.
type redisStateSupervisor struct {
	cfg      *pconf.Main
	logger   logging.LoggerInterface
	splits   *storage.ProxySplitStorageImpl
	rbs      *storage.ProxyRuleBasedSegmentsStorageImpl
	segments *storage.ProxySegmentStorageImpl
	gate     *sourceGate
	opts     *commonconf.RedisConfig
	delay    func(time.Duration) time.Duration

	writer        atomic.Pointer[redisstate.Writer]
	pendingResync atomic.Bool

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{} // closed when the retry goroutine has returned, or at once when none was started

	mtx     sync.Mutex
	lastErr string
}

// attach builds the writer on client, resets Redis when force-fresh-startup is set, and starts it. The facades point at
// the writer before Start, so a failed Start leaves them attached and the caller must detach them.
func (s *redisStateSupervisor) attach(client *redis.PrefixedRedisClient) (*redisstate.Writer, bool, error) {
	redisCfg := s.cfg.Storage.Redis
	keys, err := redisstate.NewKeys(client.Prefix(), s.cfg.Apikey, redisCfg.Namespace)
	if err != nil {
		return nil, false, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}

	options := redisstate.DefaultOptions()
	options.Retention = time.Duration(redisCfg.RetentionHours) * time.Hour
	options.MaxAttempts = int(redisCfg.WriteMaxAttempts)
	options.BackoffMaxWait = time.Duration(redisCfg.WriteBackoffMaxMs) * time.Millisecond
	options.MaxPendingChanges = int(redisCfg.MaxPendingChanges)
	options.ChunkSize = int(redisCfg.ChunkSize)
	options.PurgeBatch = int(redisCfg.PurgeBatch)
	options.StopTimeoutMs = redisCfg.StopTimeoutMs

	fingerprint := util.ConfigFingerprint(s.cfg.Apikey, s.cfg.FlagSpecVersion, s.cfg.FlagSetsFilter)
	writer, err := redisstate.NewWriter(client, keys, fingerprint, options, s.logger)
	if err != nil {
		return nil, false, common.NewInitError(fmt.Errorf("error creating redis state writer: %w", err), common.ExitInvalidConfiguration)
	}
	s.logger.Info(fmt.Sprintf("redis state writer shutdown wait bound %s (drain %d ms, per-call timeouts %d+%d+%d s, retries %d)",
		stopWaitBound(redisCfg, redisCfg.Connection), redisCfg.StopTimeoutMs, redisCfg.Connection.DialTimeout,
		redisCfg.Connection.ReadTimeout, redisCfg.Connection.WriteTimeout, redisCfg.Connection.MaxRetries))

	// force-fresh-startup resets the Redis state before the first cloud sync, so if the cloud is unreachable the
	// previous Redis state is already gone and the proxy exits; this is intended (fresh start means start from nothing).
	// It runs only once Redis is reachable, because the reset needs a connection.
	if s.cfg.Initialization.ForceFreshStartup {
		if err := writer.Reset(); err != nil {
			return nil, isRedisUnavailable(err), common.NewInitError(fmt.Errorf("error resetting redis state: %w", err), common.ExitRedisInitializationFailed)
		}
	}

	// Stop already returned or is waiting: do not wire or start a writer that nobody would stop.
	select {
	case <-s.stop:
		return nil, false, errRedisStateStopped
	default:
	}

	s.splits.WithRedisWriter(writer)
	s.rbs.WithRedisWriter(writer)
	s.segments.WithRedisWriter(writer)

	if err := writer.Start(s.gate.wrap(storage.NewRedisStateSource(s.splits, s.rbs, s.segments))); err != nil {
		return nil, isRedisUnavailable(err), err
	}
	return writer, false, nil
}

// detach removes the writer from the storages so they stop queuing changes for a writer that never started.
func (s *redisStateSupervisor) detach() {
	s.splits.WithRedisWriter(nil)
	s.rbs.WithRedisWriter(nil)
	s.segments.WithRedisWriter(nil)
}

// retryLoop reconnects with backoff until Redis answers, Stop is called, or a non-connection error ends it.
func (s *redisStateSupervisor) retryLoop() {
	defer close(s.done)
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error(fmt.Sprintf("redis state: retry loop stopped by a panic: %v", r))
			s.setLastErr(fmt.Sprintf("redis state writer stopped: %v", r))
		}
	}()

	bo := backoff.New(2, time.Duration(s.cfg.Storage.Redis.WriteBackoffMaxMs)*time.Millisecond)
	for attempt := 1; ; attempt++ {
		timer := time.NewTimer(s.delay(bo.Next()))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
		}

		client, err := newRedisStateClient(s.opts, s.logger)
		if err != nil {
			if !isRedisUnavailable(err) {
				s.logger.Error(fmt.Sprintf("redis state writer not started, the Redis client failed: %v", err))
				s.setLastErr(fmt.Sprintf("redis state writer stopped: %v", err))
				return
			}
			s.logger.Debug(fmt.Sprintf("redis state: attempt %d, Redis still unreachable: %v", attempt, err))
			if attempt%10 == 0 {
				s.logger.Warning(fmt.Sprintf("redis state: Redis still unreachable after %d attempts, retrying: %v", attempt, err))
			}
			continue
		}

		select {
		case <-s.stop:
			return
		default:
		}

		writer, retry, err := s.attach(client)
		if err != nil {
			s.detach()
			if errors.Is(err, errRedisStateStopped) {
				return
			}
			if retry {
				// Redis dropped between the connect and the reset or start: connect again.
				s.logger.Debug(fmt.Sprintf("redis state: attempt %d, Redis went away while starting the writer: %v", attempt, err))
				continue
			}
			s.logger.Error(fmt.Sprintf("redis state writer not started after Redis became reachable: %v", err))
			s.setLastErr(fmt.Sprintf("redis state writer stopped: %v", err))
			return
		}
		s.writer.Store(writer)
		// A resync requested before the writer existed is replayed here; one requested after sees the stored writer.
		if s.pendingResync.Load() {
			writer.RequestResync()
		}
		s.setLastErr("")
		s.logger.Info("redis state writer attached after Redis became reachable")
		return
	}
}

func (s *redisStateSupervisor) setLastErr(msg string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.lastErr = msg
}

// Status reports the writer status once attached. While Redis is unreachable it reports unhealthy with the reason.
func (s *redisStateSupervisor) Status() redisstate.Status {
	if w := s.writer.Load(); w != nil {
		return w.Status()
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return redisstate.Status{LastError: s.lastErr}
}

// Writer returns the attached writer, or nil while Redis is unreachable.
func (s *redisStateSupervisor) Writer() *redisstate.Writer {
	return s.writer.Load()
}

// RequestResync asks for a full resync now, or as soon as the writer attaches.
func (s *redisStateSupervisor) RequestResync() {
	s.pendingResync.Store(true)
	if w := s.writer.Load(); w != nil {
		w.RequestResync()
	}
}

// Stop ends the retry loop, waits for it to return, then stops the writer if one was attached.
func (s *redisStateSupervisor) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
	if w := s.writer.Load(); w != nil {
		w.Stop()
	}
}

// sourceGate keeps the Redis writer from reading a partial local state. On a cold boot the storages fill while the
// first sync runs, and a resync scheduled in that window (the startup one, or one after a queue overflow) would see
// only part of the segments and delete the rest from Redis. While the gate is closed the source reads as empty, and the
// writer treats an empty source as no evidence of removal. It is opened when the sync reports ready.
type sourceGate struct{ ready atomic.Bool }

func newSourceGate(ready bool) *sourceGate {
	g := &sourceGate{}
	g.ready.Store(ready)
	return g
}

func (g *sourceGate) open() {
	if g != nil {
		g.ready.Store(true)
	}
}

func (g *sourceGate) isOpen() bool { return g == nil || g.ready.Load() }

// wrap returns src behind the gate. A nil gate returns src unchanged.
func (g *sourceGate) wrap(src redisstate.Source) redisstate.Source {
	if g == nil {
		return src
	}
	return &gatedSource{Source: src, gate: g}
}

type gatedSource struct {
	redisstate.Source
	gate *sourceGate
}

func (s *gatedSource) Flags() ([]dtos.SplitDTO, int64, error) {
	if !s.gate.isOpen() {
		return nil, 0, nil
	}
	return s.Source.Flags()
}

func (s *gatedSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	if !s.gate.isOpen() {
		return nil, 0, nil
	}
	return s.Source.RuleBasedSegments()
}

func (s *gatedSource) Segments(visit func(redisstate.SegmentState) error) error {
	if !s.gate.isOpen() {
		return nil
	}
	return s.Source.Segments(visit)
}
