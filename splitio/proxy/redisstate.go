package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"

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
	// redisStateMinLeaseTtlMs is 3 x the lease call bound. It is kept independent of redis.connection timeouts.
	redisStateMinLeaseTtlMs = int64(3 * redisstate.LeaseCallBound / time.Millisecond)
	// redisStateMaxLeaseTtlMs keeps the ms to Duration conversion far from int64 overflow.
	redisStateMaxLeaseTtlMs     = 3600000
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
	case cfg.LeaseTtlMs < redisStateMinLeaseTtlMs || cfg.LeaseTtlMs > redisStateMaxLeaseTtlMs:
		return invalid("leaseTtlMs must be %d..%d, got %d", redisStateMinLeaseTtlMs, redisStateMaxLeaseTtlMs, cfg.LeaseTtlMs)
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

// validateOfflineRedisSettings checks what offline mode needs from the Redis state settings when Redis is enabled.
// A snapshot is optional here because Redis can supply the state.
func validateOfflineRedisSettings(cfg *pconf.Main) error {
	if !cfg.Offline || !cfg.Storage.Redis.Enabled {
		return nil
	}
	if err := validateRedisStateSettings(&cfg.Storage.Redis); err != nil {
		return err
	}
	if cfg.Storage.Redis.Namespace == "" || cfg.Storage.Redis.OfflineFingerprint == "" {
		return common.NewInitError(fmt.Errorf("invalid redis state configuration: offline mode needs <redis-state-namespace | redis-state-offline-fingerprint> when redis state is enabled"), common.ExitInvalidConfiguration)
	}
	return nil
}

var newRedisStateClient = commonredis.NewRedisClient

// setupRedisState builds the Redis writer supervisor that mirrors the proxy state. It returns (nil, nil) when the
// feature is disabled. Configuration errors and non-connection client errors are returned synchronously. The supervisor
// competes for the writer lease only after the gate opens (the first cloud sync completed); a proxy that does not hold
// the lease never writes and never resets Redis. A nil gate counts as open.
func setupRedisState(cfg *pconf.Main, logger logging.LoggerInterface, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segments *storage.ProxySegmentStorageImpl, gate *sourceGate) (*redisStateSupervisor, error) {
	if !cfg.Storage.Redis.Enabled {
		return nil, nil
	}

	redisCfg := cfg.Storage.Redis
	if err := validateRedisStateSettings(&redisCfg); err != nil {
		return nil, err
	}
	// The namespace is checked here, before any connection, so a bad value never reaches Redis. The prefix is
	// only known once the client exists and is validated by the NewKeys call in the role loop.
	if _, err := redisstate.NewKeys("", cfg.Apikey, redisCfg.Namespace); err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}

	opts, err := common.ParseRedisOptions(&redisCfg.Connection)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error parsing redis state config: %w", err), common.ExitInvalidConfiguration)
	}
	lease, err := redisstate.NewLeaseClient(opts)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state lease client: %w", err), common.ExitInvalidConfiguration)
	}

	s := newRedisStateSupervisor(cfg, logger, splits, rbs, segments, gate, roleDeps{
		clock:     realClock{},
		lease:     lease,
		newClient: func() (*redis.PrefixedRedisClient, error) { return newRedisStateClient(opts, logger) },
		newWriter: productionWriterFactory(logger),
	})

	// The go-toolkit PrefixedRedisClient has no Close, so on a later setup failure this client is not closed; process exit releases it.
	// An unreachable Redis is not fatal: the role loop builds the client again when it first needs it.
	client, err := s.deps.newClient()
	switch {
	case err == nil:
		s.redisClient = client
	case !isRedisUnavailable(err):
		_ = lease.Close()
		return nil, common.NewInitError(fmt.Errorf("error instantiating redis state client: %w", err), common.ExitRedisInitializationFailed)
	default:
		logger.Error(fmt.Sprintf("redis state: Redis is not reachable, the proxy keeps running and retries in the background: %v", err))
	}
	go s.run()
	return s, nil
}

// newRedisStateSupervisor builds a supervisor without starting its role loop.
func newRedisStateSupervisor(cfg *pconf.Main, logger logging.LoggerInterface, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segments *storage.ProxySegmentStorageImpl, gate *sourceGate, deps roleDeps) *redisStateSupervisor {
	s := &redisStateSupervisor{
		cfg:          cfg,
		logger:       logger,
		splits:       splits,
		rbs:          rbs,
		segments:     segments,
		gate:         gate,
		deps:         deps,
		owner:        newLeaseOwner(),
		leaseTTL:     time.Duration(cfg.Storage.Redis.LeaseTtlMs) * time.Millisecond,
		resetPending: cfg.Initialization.ForceFreshStartup,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
	s.logger.Info(fmt.Sprintf("redis state writer shutdown wait bound %s (drain %d ms, per-call timeouts %d+%d+%d s, retries %d)",
		stopWaitBound(cfg.Storage.Redis, cfg.Storage.Redis.Connection), cfg.Storage.Redis.StopTimeoutMs, cfg.Storage.Redis.Connection.DialTimeout,
		cfg.Storage.Redis.Connection.ReadTimeout, cfg.Storage.Redis.Connection.WriteTimeout, cfg.Storage.Redis.Connection.MaxRetries))
	return s
}

// newLeaseOwner returns an id that is stable for this process and unique across proxies.
func newLeaseOwner() string {
	host, _ := os.Hostname()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b))
}

// isRedisUnavailable reports whether err means Redis could not be reached, as opposed to a configuration or protocol error.
func isRedisUnavailable(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded)
}

// redisStateSupervisor owns the writer role for the lifetime of the proxy (D10). The role loop in writerrole.go
// acquires the lease after the gate opens, runs one Writer per term and steps down when the lease is lost.
type redisStateSupervisor struct {
	cfg      *pconf.Main
	logger   logging.LoggerInterface
	splits   *storage.ProxySplitStorageImpl
	rbs      *storage.ProxyRuleBasedSegmentsStorageImpl
	segments *storage.ProxySegmentStorageImpl
	gate     *sourceGate
	deps     roleDeps
	owner    string
	leaseTTL time.Duration

	// Owned by the role loop goroutine (the term's start goroutine writes resetPending, and the loop reads it only
	// after receiving that goroutine's result).
	redisClient  *redis.PrefixedRedisClient
	resetPending bool
	newTerm      bool
	lastRenew    time.Time

	writer atomic.Pointer[writerHolder]

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{} // closed when the role loop has returned

	mtx      sync.Mutex
	lastErr  string
	mismatch bool
}

// detach removes the writer from the storages so they stop queuing changes for a writer that is gone.
func (s *redisStateSupervisor) detach() {
	s.splits.WithRedisWriter(nil)
	s.rbs.WithRedisWriter(nil)
	s.segments.WithRedisWriter(nil)
}

// Status reports the writer status while this proxy is the writer. Otherwise it reports unhealthy with the last lease
// error, and EnvironmentMismatch when the lease was refused for a foreign fingerprint (D10).
func (s *redisStateSupervisor) Status() redisstate.Status {
	if h := s.writer.Load(); h != nil {
		return h.w.Status()
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return redisstate.Status{LastError: s.lastErr, EnvironmentMismatch: s.mismatch}
}

// Writer returns the active Writer, or nil while this proxy is not the writer.
func (s *redisStateSupervisor) Writer() *redisstate.Writer {
	if h := s.writer.Load(); h != nil {
		w, _ := h.w.(*redisstate.Writer)
		return w
	}
	return nil
}

// RequestResync asks the active Writer for a full resync. A proxy that is not the writer has nothing to resync; the
// Writer of its next term resyncs on Start.
func (s *redisStateSupervisor) RequestResync() {
	if h := s.writer.Load(); h != nil {
		h.w.RequestResync()
	}
}

// Stop ends the role loop: the Writer drains, then the lease is released.
func (s *redisStateSupervisor) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

// sourceGate keeps the Redis writer from reading a partial local state. On a cold boot the storages fill while the
// first sync runs, and a resync scheduled in that window (the startup one, or one after a queue overflow) would see
// only part of the segments and delete the rest from Redis. While the gate is closed the source reads as empty, and the
// writer treats an empty source as no evidence of removal. It is opened when the sync reports ready.
type sourceGate struct {
	ready  atomic.Bool
	opened chan struct{}
	once   sync.Once
}

func newSourceGate(ready bool) *sourceGate {
	g := &sourceGate{opened: make(chan struct{})}
	if ready {
		g.open()
	}
	return g
}

func (g *sourceGate) open() {
	if g != nil {
		g.ready.Store(true)
		g.once.Do(func() { close(g.opened) })
	}
}

// wait returns a channel that is closed once the gate is open. A nil gate is open.
func (g *sourceGate) wait() <-chan struct{} {
	if g == nil {
		c := make(chan struct{})
		close(c)
		return c
	}
	return g.opened
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
