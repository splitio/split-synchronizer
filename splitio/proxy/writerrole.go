package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

// roleTimer is the subset of time.Timer the role loop uses, so tests can drive time.
type roleTimer interface {
	C() <-chan time.Time
	Stop() bool
}

// roleClock is the time source of the role loop (D10). Tests inject a manual clock; production uses the real one.
type roleClock interface {
	Now() time.Time
	NewTimer(d time.Duration) roleTimer
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) roleTimer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ *time.Timer }

func (t realTimer) C() <-chan time.Time { return t.Timer.C }

// leaseAPI is what the role loop needs from redisstate.LeaseClient.
type leaseAPI interface {
	Acquire(ctx context.Context, keys redisstate.Keys, owner, fingerprint string, ttl time.Duration, newTerm, forceFresh bool) (int64, error)
	Renew(ctx context.Context, keys redisstate.Keys, owner string, ttl time.Duration) (int64, error)
	Release(ctx context.Context, keys redisstate.Keys, owner string, token int64) error
	Close() error
}

// roleWriter is the Writer surface the role loop and the storage facades use. *redisstate.Writer implements it.
type roleWriter interface {
	redisstate.FlagWriter
	redisstate.RuleBasedSegmentWriter
	redisstate.SegmentWriter
	Start(source redisstate.Source) error
	Stop()
	Abort()
	Reset() error
	Dead() bool
	Status() redisstate.Status
	RequestResync()
}

type writerHolder struct{ w roleWriter }

// roleDeps are the seams of the role loop. setupRedisState fills the production values.
type roleDeps struct {
	clock     roleClock
	lease     leaseAPI
	newClient func() (*redis.PrefixedRedisClient, error)
	newWriter func(client *redis.PrefixedRedisClient, keys redisstate.Keys, fingerprint string, opts redisstate.Options) (roleWriter, error)
}

func productionWriterFactory(logger logging.LoggerInterface) func(*redis.PrefixedRedisClient, redisstate.Keys, string, redisstate.Options) (roleWriter, error) {
	return func(client *redis.PrefixedRedisClient, keys redisstate.Keys, fingerprint string, opts redisstate.Options) (roleWriter, error) {
		w, err := redisstate.NewWriter(client, keys, fingerprint, opts, logger)
		if err != nil {
			return nil, err
		}
		return w, nil
	}
}

// setNonWriter records why this proxy is not the writer. The line is logged once per state change (H-3).
func (s *redisStateSupervisor) setNonWriter(reason string, mismatch bool) {
	s.mtx.Lock()
	changed := s.lastErr != reason || s.mismatch != mismatch
	s.lastErr = reason
	s.mismatch = mismatch
	s.mtx.Unlock()
	if changed {
		s.logger.Info(fmt.Sprintf("redis state: %s", reason))
	}
}

func (s *redisStateSupervisor) stopping() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

// sleep waits d on the role clock. It returns false when Stop was called first.
func (s *redisStateSupervisor) sleep(d time.Duration) bool {
	t := s.deps.clock.NewTimer(d)
	defer t.Stop()
	select {
	case <-s.stop:
		return false
	case <-t.C():
		return true
	}
}

// run is the role loop (D10). It starts acquiring only after the source gate opened (D11, INV-5).
func (s *redisStateSupervisor) run() {
	defer close(s.done)
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error(fmt.Sprintf("redis state: role loop stopped by a panic: %v", r))
			s.setNonWriter(fmt.Sprintf("not writer: role loop stopped: %v", r), false)
		}
	}()
	defer func() {
		if err := s.deps.lease.Close(); err != nil {
			s.logger.Debug(fmt.Sprintf("redis state: closing lease client: %v", err))
		}
	}()

	s.setNonWriter("not writer: waiting for first cloud sync", false)
	select {
	case <-s.stop:
		return
	case <-s.gate.wait():
	}

	retry := s.leaseTTL / 3
	for !s.stopping() {
		t, err := s.acquire()
		if err != nil {
			s.reportAcquireError(err)
			if !s.sleep(retry) {
				return
			}
			continue
		}
		s.hold(t)
		if s.stopping() {
			return
		}
		if !s.sleep(retry) {
			return
		}
	}
}

func (s *redisStateSupervisor) reportAcquireError(err error) {
	switch {
	case errors.Is(err, redisstate.ErrLeaseHeld):
		s.setNonWriter("not writer", false)
	case errors.Is(err, redisstate.ErrEnvironmentMismatch):
		s.setNonWriter("not writer: refused: foreign fingerprint", true)
	default:
		s.setNonWriter(fmt.Sprintf("not writer: %v", err), false)
	}
}

// term is one lease term: one lease token and one Writer.
type term struct {
	token    int64
	keys     redisstate.Keys
	w        roleWriter
	started  atomic.Bool
	deadline atomic.Int64 // unix nanos on the role clock
	moved    chan struct{}
	fenced   chan struct{} // closed when the self-fence timer aborted the writer
	done     chan struct{} // closed when the term ends, stops the watcher
}

// acquire builds the Redis client and the Writer for a new term, then takes the lease. A nil error means this proxy
// holds the lease at term.token. The Writer is created but not started.
func (s *redisStateSupervisor) acquire() (*term, error) {
	client, err := s.client()
	if err != nil {
		return nil, err
	}
	keys, err := redisstate.NewKeys(client.Prefix(), s.cfg.Apikey, s.cfg.Storage.Redis.Namespace)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}
	fingerprint := util.ConfigFingerprint(s.cfg.Apikey, s.cfg.FlagSpecVersion, s.cfg.FlagSetsFilter)

	sent := s.deps.clock.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	token, err := s.deps.lease.Acquire(ctx, keys, s.owner, fingerprint, s.leaseTTL, s.newTerm, s.resetPending)
	if err != nil {
		return nil, err
	}
	s.newTerm = false

	opts := s.writerOptions()
	opts.Owner = s.owner
	opts.Token = token
	w, err := s.deps.newWriter(client, keys, fingerprint, opts)
	if err != nil {
		s.releaseBestEffort(keys, token)
		s.newTerm = true
		return nil, fmt.Errorf("creating the redis state writer: %w", err)
	}
	t := &term{token: token, keys: keys, w: w, moved: make(chan struct{}, 1), fenced: make(chan struct{}), done: make(chan struct{})}
	t.deadline.Store(sent.Add(s.leaseTTL - s.leaseTTL/3).UnixNano())
	return t, nil
}

// client returns the writer client, building it the first time it is needed. An unreachable Redis is a retryable acquire error.
func (s *redisStateSupervisor) client() (*redis.PrefixedRedisClient, error) {
	if s.redisClient != nil {
		return s.redisClient, nil
	}
	c, err := s.deps.newClient()
	if err != nil {
		return nil, fmt.Errorf("redis client: %w", err)
	}
	s.redisClient = c
	return c, nil
}

func (s *redisStateSupervisor) writerOptions() redisstate.Options {
	redisCfg := s.cfg.Storage.Redis
	options := redisstate.DefaultOptions()
	options.Retention = time.Duration(redisCfg.RetentionHours) * time.Hour
	options.MaxAttempts = int(redisCfg.WriteMaxAttempts)
	options.BackoffMaxWait = time.Duration(redisCfg.WriteBackoffMaxMs) * time.Millisecond
	options.MaxPendingChanges = int(redisCfg.MaxPendingChanges)
	options.ChunkSize = int(redisCfg.ChunkSize)
	options.PurgeBatch = int(redisCfg.PurgeBatch)
	options.StopTimeoutMs = redisCfg.StopTimeoutMs
	return options
}

// watch is the independent self-fence timer (D3, INV-7): it aborts the Writer at the deadline even when the renew call
// is stuck. A renew that succeeds in time moves the deadline and wakes the watcher.
func (s *redisStateSupervisor) watch(t *term) {
	for {
		wait := time.Duration(t.deadline.Load() - s.deps.clock.Now().UnixNano())
		if wait < 0 {
			wait = 0
		}
		timer := s.deps.clock.NewTimer(wait)
		select {
		case <-t.done:
			timer.Stop()
			return
		case <-t.moved:
			timer.Stop()
		case <-timer.C():
			if s.deps.clock.Now().UnixNano() < t.deadline.Load() {
				continue
			}
			t.w.Abort()
			close(t.fenced)
			return
		}
	}
}

// renewOnce renews the lease and returns nil when the lease is still ours at this token.
func (s *redisStateSupervisor) renewOnce(t *term) (lost bool, err error) {
	sent := s.deps.clock.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fence, err := s.deps.lease.Renew(ctx, t.keys, s.owner, s.leaseTTL)
	if err != nil {
		return errors.Is(err, redisstate.ErrLeaseLost), err
	}
	if fence != t.token {
		return true, fmt.Errorf("fence is %d, this term holds %d", fence, t.token)
	}
	deadline := sent.Add(s.leaseTTL - s.leaseTTL/3)
	if s.deps.clock.Now().Before(deadline) {
		// A renew that completes at or after its own deadline does not extend the self-fence deadline.
		t.deadline.Store(deadline.UnixNano())
		select {
		case t.moved <- struct{}{}:
		default:
		}
		s.lastRenew = sent
	}
	return false, nil
}

// hold runs one term until the proxy steps down or Stop is called.
func (s *redisStateSupervisor) hold(t *term) {
	go s.watch(t)
	defer close(t.done)
	s.lastRenew = s.deps.clock.Now()

	startRes := make(chan error, 1)
	go func() { startRes <- s.startWriter(t) }()

	interval := s.leaseTTL / 3
	timer := s.deps.clock.NewTimer(interval)
	defer func() { timer.Stop() }()
	for {
		select {
		case <-s.stop:
			s.shutdown(t, startRes, timer)
			return
		case <-t.fenced:
			s.stepDown(t, startRes, "self-fence: no successful renew before the deadline", false)
			return
		case err := <-startRes:
			startRes = nil
			if err != nil {
				s.stepDown(t, nil, fmt.Sprintf("starting the writer: %v", err), true)
				return
			}
			t.started.Store(true)
		case <-timer.C():
			if reason := s.renewTick(t); reason != "" {
				s.stepDown(t, startRes, reason, true)
				return
			}
			next := interval - s.deps.clock.Now().Sub(s.lastRenew)
			if next < 0 {
				next = 0
			}
			timer = s.deps.clock.NewTimer(next)
		}
	}
}

// renewTick checks the Writer, then renews with one immediate retry. It returns the step-down reason, or "" to carry on.
func (s *redisStateSupervisor) renewTick(t *term) string {
	if t.started.Load() && t.w.Dead() {
		return "the writer stopped (worker died, environment mismatch or fenced)"
	}
	lost, err := s.renewOnce(t)
	if err == nil {
		return ""
	}
	if lost {
		return fmt.Sprintf("lease lost: %v", err)
	}
	s.logger.Warning(fmt.Sprintf("redis state: lease renew failed, retrying once: %v", err))
	if lost, err = s.renewOnce(t); err != nil {
		if lost {
			return fmt.Sprintf("lease lost: %v", err)
		}
		return fmt.Sprintf("lease renew failed twice: %v", err)
	}
	return ""
}

// startWriter runs the fenced Reset (once per process, D9), wires the facades and starts the Writer.
func (s *redisStateSupervisor) startWriter(t *term) error {
	if s.resetPending {
		if err := t.w.Reset(); err != nil {
			return fmt.Errorf("resetting redis state: %w", err)
		}
		s.resetPending = false
	}
	s.splits.WithRedisWriter(t.w)
	s.rbs.WithRedisWriter(t.w)
	s.segments.WithRedisWriter(t.w)
	s.setActive(t.w)
	if err := t.w.Start(s.gate.wrap(storage.NewRedisStateSource(s.splits, s.rbs, s.segments))); err != nil {
		return err
	}
	return nil
}

func (s *redisStateSupervisor) setActive(w roleWriter) {
	s.writer.Store(&writerHolder{w: w})
	s.mtx.Lock()
	s.lastErr = ""
	s.mismatch = false
	s.mtx.Unlock()
	s.logger.Info("redis state: this proxy is the writer")
}

// stepDown ends the term: facades detached, Writer aborted without a drain, best-effort release, next acquire is a new term (F-1).
func (s *redisStateSupervisor) stepDown(t *term, startRes <-chan error, reason string, abort bool) {
	s.logger.Warning(fmt.Sprintf("redis state: stepping down: %s", reason))
	s.detach()
	s.writer.Store(nil)
	if abort {
		t.w.Abort()
	}
	if startRes != nil {
		if err := <-startRes; err != nil {
			s.logger.Debug(fmt.Sprintf("redis state: the aborted writer start returned: %v", err))
		}
		s.detach()
	}
	// The start goroutine has returned, so setActive can no longer run. A start that was still in
	// Reset/Start when the first Store(nil) above ran has now published a writer; clear it again.
	s.writer.Store(nil)
	s.releaseBestEffort(t.keys, t.token)
	s.newTerm = true
	s.setNonWriter(fmt.Sprintf("not writer: %s", reason), false)
}

// shutdown drains the Writer while the lease keeps being renewed, then releases it (D10).
func (s *redisStateSupervisor) shutdown(t *term, startRes <-chan error, timer roleTimer) {
	s.detach()
	if startRes != nil {
		// Reset or Start still running: nothing worth draining.
		t.w.Abort()
		if err := <-startRes; err != nil {
			s.logger.Debug(fmt.Sprintf("redis state: the aborted writer start returned: %v", err))
		}
		s.detach()
		s.releaseBestEffort(t.keys, t.token)
		s.writer.Store(nil)
		return
	}
	drained := make(chan struct{})
	go func() {
		t.w.Stop()
		close(drained)
	}()
	fenced := t.fenced
	for {
		select {
		case <-drained:
			s.releaseBestEffort(t.keys, t.token)
			s.writer.Store(nil)
			return
		case <-fenced:
			// The self-fence timer aborted the writer, so Stop returns at once.
			fenced = nil
		case <-timer.C():
			if _, err := s.renewOnce(t); err != nil {
				s.logger.Warning(fmt.Sprintf("redis state: lease renew during the shutdown drain failed: %v", err))
			}
			timer = s.deps.clock.NewTimer(s.leaseTTL / 3)
		}
	}
}

// releaseBestEffort deletes the lease when it is still ours at this token. The error is logged and ignored.
func (s *redisStateSupervisor) releaseBestEffort(keys redisstate.Keys, token int64) {
	if err := s.deps.lease.Release(context.Background(), keys, s.owner, token); err != nil {
		s.logger.Warning(fmt.Sprintf("redis state: releasing the lease failed, it expires on its own: %v", err))
	}
}
