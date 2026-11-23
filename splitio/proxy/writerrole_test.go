package proxy

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

// settle lets the goroutines woken by the last clock move run before the test looks at their effects.
func settle() {
	for i := 0; i < 5; i++ {
		runtime.Gosched()
	}
	time.Sleep(3 * time.Millisecond)
}

// eventually polls cond for up to 3s of real time. It does not move the fake clock.
func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", msg)
		}
		time.Sleep(time.Millisecond)
	}
}

// fakeClock is a manual roleClock. Timers fire only from Advance, in due order.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	seq    uint64
	timers []*fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) roleTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	tm := &fakeTimer{clock: c, when: c.now.Add(d), seq: c.seq, ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, tm)
	return tm
}

// Advance moves the clock by d and fires every timer that comes due, in order. Goroutines get a chance to run after each fire.
func (c *fakeClock) Advance(d time.Duration) {
	settle()
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *fakeTimer
		for _, tm := range c.timers {
			if tm.when.After(target) {
				continue
			}
			if next == nil || tm.when.Before(next.when) || (tm.when.Equal(next.when) && tm.seq < next.seq) {
				next = tm
			}
		}
		if next == nil {
			c.now = target
			c.mu.Unlock()
			break
		}
		c.removeLocked(next)
		if next.when.After(c.now) {
			c.now = next.when
		}
		now := c.now
		c.mu.Unlock()
		next.ch <- now
		settle()
	}
	settle()
}

func (c *fakeClock) removeLocked(tm *fakeTimer) bool {
	for i, x := range c.timers {
		if x == tm {
			c.timers = append(c.timers[:i], c.timers[i+1:]...)
			return true
		}
	}
	return false
}

type fakeTimer struct {
	clock *fakeClock
	when  time.Time
	seq   uint64
	ch    chan time.Time
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	return t.clock.removeLocked(t)
}

// gate is a one-shot release signal.
type gate struct {
	ch   chan struct{}
	once sync.Once
}

func newGate() *gate { return &gate{ch: make(chan struct{})} }

func (g *gate) open() { g.once.Do(func() { close(g.ch) }) }

type acquireCall struct {
	owner, fingerprint  string
	newTerm, forceFresh bool
	token               int64
	err                 error
}

type releaseCall struct {
	owner string
	token int64
}

// fakeLease is a scriptable leaseAPI. Each method pops one scripted error per call, when one is queued.
type fakeLease struct {
	mu          sync.Mutex
	token       int64
	acquireErrs []error
	renewErrs   []error
	releaseErrs []error
	renewGate   *gate
	panicRenew  bool
	gates       []*gate
	acquires    []acquireCall
	renews      int
	releases    []releaseCall
	closes      int
}

func (f *fakeLease) Acquire(_ context.Context, _ redisstate.Keys, owner, fingerprint string, _ time.Duration, newTerm, forceFresh bool) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := acquireCall{owner: owner, fingerprint: fingerprint, newTerm: newTerm, forceFresh: forceFresh}
	if len(f.acquireErrs) > 0 {
		call.err = f.acquireErrs[0]
		f.acquireErrs = f.acquireErrs[1:]
	}
	if call.err == nil {
		f.token++
		call.token = f.token
	}
	f.acquires = append(f.acquires, call)
	return call.token, call.err
}

func (f *fakeLease) Renew(_ context.Context, _ redisstate.Keys, _ string, _ time.Duration) (int64, error) {
	f.mu.Lock()
	f.renews++
	g := f.renewGate
	f.renewGate = nil
	boom := f.panicRenew
	f.mu.Unlock()
	if boom {
		panic("injected renew panic")
	}
	if g != nil {
		<-g.ch
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.renewErrs) > 0 {
		err := f.renewErrs[0]
		f.renewErrs = f.renewErrs[1:]
		return 0, err
	}
	return f.token, nil
}

func (f *fakeLease) Release(_ context.Context, _ redisstate.Keys, owner string, token int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases = append(f.releases, releaseCall{owner: owner, token: token})
	if len(f.releaseErrs) > 0 {
		err := f.releaseErrs[0]
		f.releaseErrs = f.releaseErrs[1:]
		return err
	}
	return nil
}

func (f *fakeLease) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

func (f *fakeLease) failAcquire(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireErrs = append(f.acquireErrs, errs...)
}

func (f *fakeLease) failRenew(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewErrs = append(f.renewErrs, errs...)
}

// blockNextRenew makes the next Renew call hang until the returned gate is opened.
func (f *fakeLease) blockNextRenew() *gate {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := newGate()
	f.renewGate = g
	f.gates = append(f.gates, g)
	return g
}

func (f *fakeLease) openGates() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.gates {
		g.open()
	}
}

func (f *fakeLease) acquireCalls() []acquireCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]acquireCall(nil), f.acquires...)
}

func (f *fakeLease) acquireCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acquires)
}

func (f *fakeLease) renewCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.renews
}

func (f *fakeLease) releaseCalls() []releaseCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]releaseCall(nil), f.releases...)
}

func (f *fakeLease) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// writerScript holds the Reset script shared by every fakeWriter of a supervisor.
type writerScript struct {
	mu        sync.Mutex
	resetErrs []error
	resets    int
	hold      *gate
}

// holdReset makes Reset calls hang until the next Abort on any fakeWriter of this script.
func (s *writerScript) holdReset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold = newGate()
}

func (s *writerScript) releaseHold() {
	s.mu.Lock()
	g := s.hold
	s.mu.Unlock()
	if g != nil {
		g.open()
	}
}

func (s *writerScript) nextReset() error {
	s.mu.Lock()
	s.resets++
	hold := s.hold
	s.mu.Unlock()
	if hold != nil {
		<-hold.ch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.resetErrs) > 0 {
		err := s.resetErrs[0]
		s.resetErrs = s.resetErrs[1:]
		return err
	}
	return nil
}

func (s *writerScript) failReset(errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetErrs = append(s.resetErrs, errs...)
}

func (s *writerScript) resetCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resets
}

// fakeWriter is a roleWriter that records calls. Stop can be made to block with blockStop.
type fakeWriter struct {
	opts       redisstate.Options
	script     *writerScript
	startCalls atomic.Int32
	stopCalls  atomic.Int32
	aborts     atomic.Int32
	resyncs    atomic.Int32
	dead       atomic.Bool

	mu          sync.Mutex
	startSource redisstate.Source
	stopGate    *gate
}

func (w *fakeWriter) PutFlags(_, _ []dtos.SplitDTO, _ int64)                        {}
func (w *fakeWriter) PutRuleBasedSegments(_, _ []dtos.RuleBasedSegmentDTO, _ int64) {}
func (w *fakeWriter) PutSegment(_ string, _, _ []string, _ int64)                   {}

func (w *fakeWriter) Start(source redisstate.Source) error {
	w.mu.Lock()
	w.startSource = source
	w.mu.Unlock()
	w.startCalls.Add(1)
	return nil
}

func (w *fakeWriter) Stop() {
	w.stopCalls.Add(1)
	w.mu.Lock()
	g := w.stopGate
	w.mu.Unlock()
	if g != nil {
		<-g.ch
	}
}

func (w *fakeWriter) Abort()                    { w.aborts.Add(1); w.script.releaseHold() }
func (w *fakeWriter) Reset() error              { return w.script.nextReset() }
func (w *fakeWriter) Dead() bool                { return w.dead.Load() }
func (w *fakeWriter) Status() redisstate.Status { return redisstate.Status{Healthy: true} }
func (w *fakeWriter) RequestResync()            { w.resyncs.Add(1) }

func (w *fakeWriter) source() redisstate.Source {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.startSource
}

// blockStop makes the next Stop call hang until the returned gate is opened.
func (w *fakeWriter) blockStop() *gate {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopGate = newGate()
	return w.stopGate
}

// syncBuffer is a logger sink safe for concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// roleHarness is a supervisor wired to fakes: a manual clock, a scripted lease and fake Writers.
type roleHarness struct {
	t       *testing.T
	sup     *redisStateSupervisor
	clock   *fakeClock
	lease   *fakeLease
	gate    *sourceGate
	script  *writerScript
	logs    *syncBuffer
	mu      sync.Mutex
	writers []*fakeWriter
}

// newRoleHarness builds a supervisor with a 6s lease (renew and retry every 2s on the fake clock) and starts its role loop.
func newRoleHarness(t *testing.T, forceFresh, gateOpen bool) *roleHarness {
	t.Helper()
	cfg := redisStateTestConfig(t)
	cfg.Storage.Redis.LeaseTtlMs = 6000
	cfg.Initialization.ForceFreshStartup = forceFresh

	logs := &syncBuffer{}
	logger := logging.NewLogger(&logging.LoggerOptions{
		LogLevel:      logging.LevelInfo,
		ErrorWriter:   logs,
		WarningWriter: logs,
		InfoWriter:    logs,
		DebugWriter:   logs,
		VerboseWriter: logs,
	})
	db, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "state.db"), nil)
	require.NoError(t, err)
	splits := storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(nil), false)
	rbs := storage.NewProxyRuleBasedSegmentsStorage(db, logger, false)
	segments := storage.NewProxySegmentStorage(db, logger, false)

	raw, err := redis.NewClient(&redis.UniversalOptions{Addrs: []string{"localhost:6379"}})
	require.NoError(t, err)
	client, err := redis.NewPrefixedRedisClient(raw, "")
	require.NoError(t, err)

	h := &roleHarness{
		t:      t,
		clock:  newFakeClock(),
		lease:  &fakeLease{},
		gate:   newSourceGate(gateOpen),
		script: &writerScript{},
		logs:   logs,
	}
	h.sup = newRedisStateSupervisor(cfg, logger, splits, rbs, segments, h.gate, roleDeps{
		clock: h.clock,
		lease: h.lease,
		newClient: func() (*redis.PrefixedRedisClient, error) {
			return client, nil
		},
		newWriter: func(_ *redis.PrefixedRedisClient, _ redisstate.Keys, _ string, opts redisstate.Options) (roleWriter, error) {
			return h.newWriter(opts), nil
		},
	})
	t.Cleanup(func() {
		h.lease.openGates()
		h.mu.Lock()
		ws := append([]*fakeWriter(nil), h.writers...)
		h.mu.Unlock()
		for _, w := range ws {
			w.mu.Lock()
			g := w.stopGate
			w.mu.Unlock()
			if g != nil {
				g.open()
			}
		}
		stopped := make(chan struct{})
		go func() {
			h.sup.Stop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Errorf("supervisor Stop did not return within 3s")
		}
	})
	go h.sup.run()
	return h
}

func (h *roleHarness) newWriter(opts redisstate.Options) *fakeWriter {
	w := &fakeWriter{opts: opts, script: h.script}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writers = append(h.writers, w)
	return w
}

func (h *roleHarness) writerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.writers)
}

func (h *roleHarness) writerAt(i int) *fakeWriter {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writers[i]
}

// awaitStarted waits until n writers exist and the newest one has been started, then lets the role loop settle.
func (h *roleHarness) awaitStarted(n int) {
	h.t.Helper()
	eventually(h.t, func() bool {
		return h.writerCount() >= n && h.writerAt(n-1).startCalls.Load() >= 1
	}, "writer started")
	settle()
}

// active returns the Writer the supervisor currently exposes to the storages, or nil.
func (h *roleHarness) active() roleWriter {
	if hld := h.sup.writer.Load(); hld != nil {
		return hld.w
	}
	return nil
}

// logCount counts log lines that end with msg.
func (h *roleHarness) logCount(msg string) int {
	n := 0
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), msg) {
			n++
		}
	}
	return n
}

func TestWriterRoleSelfFencesOnRenewFailure(t *testing.T) {
	t.Run("one failed renew is retried and keeps the writer", func(t *testing.T) {
		h := newRoleHarness(t, false, true)
		h.awaitStarted(1)
		h.lease.failRenew(errors.New("transient"))
		h.clock.Advance(2 * time.Second)
		eventually(t, func() bool { return h.lease.renewCount() == 2 }, "renew retried")
		settle()
		assert.Equal(t, int32(0), h.writerAt(0).aborts.Load())
		assert.Empty(t, h.lease.releaseCalls())
		assert.Equal(t, 1, h.writerCount())
	})

	t.Run("two failed renews step down, then re-acquire a higher term with a new writer", func(t *testing.T) {
		h := newRoleHarness(t, false, true)
		h.awaitStarted(1)
		first := h.writerAt(0)
		h.lease.failRenew(errors.New("transient"), errors.New("transient"))
		h.clock.Advance(2 * time.Second)
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "lease released on step down")
		assert.Equal(t, int32(1), first.aborts.Load())
		rel := h.lease.releaseCalls()[0]
		assert.Equal(t, h.sup.owner, rel.owner)
		assert.Equal(t, int64(1), rel.token)
		assert.Nil(t, h.active())

		h.clock.Advance(2 * time.Second)
		h.awaitStarted(2)
		acq := h.lease.acquireCalls()
		last := acq[len(acq)-1]
		assert.True(t, last.newTerm)
		assert.Equal(t, int64(2), last.token)
		second := h.writerAt(1)
		assert.NotSame(t, first, second)
		assert.Equal(t, int64(2), second.opts.Token)
		assert.Equal(t, h.sup.owner, second.opts.Owner)
		assert.Equal(t, int32(0), second.aborts.Load())
	})

	t.Run("hung renew: the self-fence timer aborts the writer before the renew returns", func(t *testing.T) {
		h := newRoleHarness(t, false, true)
		h.awaitStarted(1)
		w := h.writerAt(0)
		hung := h.lease.blockNextRenew()
		h.clock.Advance(2 * time.Second)
		eventually(t, func() bool { return h.lease.renewCount() == 1 }, "renew in flight")
		h.clock.Advance(2 * time.Second) // T0+4, the acquire deadline
		eventually(t, func() bool { return w.aborts.Load() == 1 }, "self-fence abort while renew hangs")
		assert.Empty(t, h.lease.releaseCalls(), "no step down while the renew is still in flight")

		hung.open()
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down after renew returns")
		assert.Equal(t, int32(1), w.aborts.Load())
	})

	t.Run("renew that succeeds only at its deadline does not extend the fence", func(t *testing.T) {
		h := newRoleHarness(t, false, true)
		h.awaitStarted(1)
		w := h.writerAt(0)
		late := h.lease.blockNextRenew()
		h.clock.Advance(2 * time.Second) // renew sent at T0+2, its deadline is T0+6
		eventually(t, func() bool { return h.lease.renewCount() == 1 }, "renew in flight")
		h.clock.Advance(4 * time.Second) // T0+6: the watch aborts at T0+4 on the way
		eventually(t, func() bool { return w.aborts.Load() == 1 }, "writer aborted at the deadline")

		late.open()
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down after late renew")
		assert.Equal(t, int32(1), w.aborts.Load())
		assert.Equal(t, int64(1), h.lease.releaseCalls()[0].token)
	})
}

func TestWriterRoleStop(t *testing.T) {
	h := newRoleHarness(t, false, true)
	h.awaitStarted(1)
	w := h.writerAt(0)
	drain := w.blockStop()

	stopped := make(chan struct{})
	go func() {
		h.sup.Stop()
		close(stopped)
	}()
	eventually(t, func() bool { return w.stopCalls.Load() == 1 }, "writer drain started")

	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return h.lease.renewCount() == 1 }, "renew during drain")
	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return h.lease.renewCount() == 2 }, "renew keeps running during drain")
	assert.Empty(t, h.lease.releaseCalls(), "lease must be held until the writer Stop returns")
	assert.Equal(t, 0, h.lease.closeCount())

	drain.open()
	eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "lease released after drain")
	rel := h.lease.releaseCalls()[0]
	assert.Equal(t, h.sup.owner, rel.owner)
	assert.Equal(t, int64(1), rel.token)

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor Stop did not return within 3s")
	}
	eventually(t, func() bool { return h.lease.closeCount() == 1 }, "lease client closed")
}

func TestWriterRoleNonWriterLogs(t *testing.T) {
	h := newRoleHarness(t, false, true)
	h.lease.failAcquire(redisstate.ErrLeaseHeld, redisstate.ErrLeaseHeld, redisstate.ErrLeaseHeld, redisstate.ErrLeaseHeld)
	eventually(t, func() bool { return h.lease.acquireCount() == 1 }, "first acquire")

	for i := 0; i < 3; i++ {
		h.clock.Advance(2 * time.Second) // TTL/3
		want := i + 2
		eventually(t, func() bool { return h.lease.acquireCount() == want }, "retry acquire")
	}
	settle()
	assert.Equal(t, 4, h.lease.acquireCount())
	assert.Equal(t, 0, h.writerCount(), "no writer is built while the lease is held elsewhere")
	assert.Equal(t, 1, h.logCount("redis state: not writer"), "the not-writer line is logged once")
}

func TestWriterRoleResetPending(t *testing.T) {
	t.Run("forced fresh startup carries forceFresh until Reset succeeds", func(t *testing.T) {
		h := newRoleHarness(t, true, true)
		h.script.failReset(errors.New("reset boom"))

		eventually(t, func() bool { return h.lease.acquireCount() == 1 }, "first acquire")
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down after failed Reset")
		assert.Equal(t, int32(1), h.writerAt(0).aborts.Load())
		assert.Equal(t, 1, h.script.resetCount())

		h.clock.Advance(2 * time.Second)
		h.awaitStarted(2)
		assert.Equal(t, 2, h.script.resetCount())

		h.lease.failRenew(errors.New("transient"), errors.New("transient"))
		h.clock.Advance(2 * time.Second)
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 2 }, "step down of the second term")

		h.clock.Advance(2 * time.Second)
		h.awaitStarted(3)

		acq := h.lease.acquireCalls()
		require.Len(t, acq, 3)
		assert.True(t, acq[0].forceFresh)
		assert.True(t, acq[1].forceFresh, "Reset failed on term 1, so term 2 still forces fresh")
		assert.True(t, acq[1].newTerm)
		assert.False(t, acq[2].forceFresh, "Reset succeeded on term 2")
		assert.True(t, acq[2].newTerm)
		assert.Equal(t, 2, h.script.resetCount(), "Reset is not called again after it succeeded")
	})

	t.Run("without forced fresh startup Reset is never called", func(t *testing.T) {
		h := newRoleHarness(t, false, true)
		h.awaitStarted(1)
		h.lease.failRenew(errors.New("transient"), errors.New("transient"))
		h.clock.Advance(2 * time.Second)
		eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down")

		h.clock.Advance(2 * time.Second)
		h.awaitStarted(2)
		for i, c := range h.lease.acquireCalls() {
			assert.False(t, c.forceFresh, "acquire %d", i)
		}
		assert.Equal(t, 0, h.script.resetCount())
	})
}

func TestPromotionStartsFreshWriterAndResyncs(t *testing.T) {
	h := newRoleHarness(t, false, true)
	h.lease.failAcquire(redisstate.ErrLeaseHeld)
	eventually(t, func() bool { return h.lease.acquireCount() == 1 }, "first acquire")
	settle()
	assert.Equal(t, 0, h.writerCount(), "no writer while the lease is held elsewhere")

	h.clock.Advance(2 * time.Second)
	h.awaitStarted(1)
	w1 := h.writerAt(0)
	assert.Equal(t, int32(1), w1.startCalls.Load())
	assert.NotNil(t, w1.source(), "Start gets a non-nil source")
	assert.Equal(t, int32(0), w1.resyncs.Load(), "the supervisor does not request a resync")
	assert.Equal(t, int64(1), w1.opts.Token)

	h.lease.failRenew(errors.New("transient"), errors.New("transient"))
	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down")
	assert.Equal(t, int32(1), w1.aborts.Load())
	assert.Equal(t, int32(0), w1.stopCalls.Load(), "the old writer is aborted, not stopped")

	h.clock.Advance(2 * time.Second)
	h.awaitStarted(2)
	w2 := h.writerAt(1)
	assert.NotSame(t, w1, w2)
	assert.Equal(t, int32(1), w2.startCalls.Load())
	assert.Greater(t, w2.opts.Token, w1.opts.Token)
	assert.Equal(t, int32(0), w2.resyncs.Load())
}

func TestNonWriterStatusReportsLeaseErrorAndRefused(t *testing.T) {
	h := newRoleHarness(t, false, false)
	settle()
	assert.Equal(t, 0, h.lease.acquireCount(), "no Acquire while the source gate is closed")
	st := h.sup.Status()
	assert.False(t, st.Healthy)
	assert.Equal(t, "not writer: waiting for first cloud sync", st.LastError)
	assert.False(t, st.EnvironmentMismatch)

	h.lease.failAcquire(redisstate.ErrLeaseHeld)
	h.gate.open()
	eventually(t, func() bool { return h.lease.acquireCount() == 1 }, "acquire after the gate opened")
	eventually(t, func() bool { return h.sup.Status().LastError == "not writer" }, "lease held reported")
	assert.False(t, h.sup.Status().EnvironmentMismatch)

	h.lease.failAcquire(redisstate.ErrEnvironmentMismatch)
	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return h.sup.Status().EnvironmentMismatch }, "mismatch reported")
	assert.Contains(t, h.sup.Status().LastError, "refused")

	h.lease.failAcquire(errors.New("network melted 42"))
	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return strings.Contains(h.sup.Status().LastError, "network melted 42") }, "generic error reported")
	assert.False(t, h.sup.Status().EnvironmentMismatch)
}

func TestWriterRoleStepDownClearsWriterStartedDuringStepDown(t *testing.T) {
	// The source gate is closed so the first Reset cannot run before holdReset is set.
	h := newRoleHarness(t, true, false)
	h.script.holdReset()
	h.gate.open()
	eventually(t, func() bool { return h.script.resetCount() == 1 }, "Reset in progress")

	h.lease.failRenew(errors.New("transient"), errors.New("transient"))
	h.clock.Advance(2 * time.Second)
	eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down")
	settle()
	assert.Nil(t, h.sup.Writer(), "the writer started during step down must not stay published")
	assert.True(t, strings.HasPrefix(h.sup.Status().LastError, "not writer"))
	assert.Nil(t, h.active())
}

// TestWriterRolePanicStepsDownFully covers PR review comment 4: a panic in the role loop detaches the facades, clears
// the published writer, aborts the Writer and releases the lease, so Status does not report a live writer.
func TestWriterRolePanicStepsDownFully(t *testing.T) {
	h := newRoleHarness(t, false, true)
	h.awaitStarted(1)
	w := h.writerAt(0)
	require.NotNil(t, h.active(), "precondition: the writer is published")

	h.lease.mu.Lock()
	h.lease.panicRenew = true
	h.lease.mu.Unlock()
	h.clock.Advance(2 * time.Second) // next renew tick panics
	eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "lease released after the panic")
	eventually(t, func() bool { return w.aborts.Load() == 1 }, "writer aborted after the panic")

	assert.Nil(t, h.active(), "no live writer is published after the panic")
	assert.True(t, strings.HasPrefix(h.sup.Status().LastError, "not writer: role loop stopped"))
	assert.Equal(t, h.sup.owner, h.lease.releaseCalls()[0].owner)
	assert.Equal(t, int64(1), h.lease.releaseCalls()[0].token)
	assert.Equal(t, 1, h.lease.closeCount())
	select {
	case <-h.sup.done:
	case <-time.After(3 * time.Second):
		t.Fatal("role loop did not exit after the panic")
	}
}

func TestWriterRoleStepsDownWhenWriterDies(t *testing.T) {
	h := newRoleHarness(t, false, true)
	h.awaitStarted(1)
	w := h.writerAt(0)
	w.dead.Store(true)

	h.clock.Advance(2 * time.Second) // next renew tick
	eventually(t, func() bool { return len(h.lease.releaseCalls()) == 1 }, "step down on dead writer")
	assert.Equal(t, int32(1), w.aborts.Load())
	assert.Equal(t, int64(1), h.lease.releaseCalls()[0].token)
	assert.Contains(t, h.sup.Status().LastError, "not writer")

	h.clock.Advance(2 * time.Second)
	h.awaitStarted(2)
	assert.Equal(t, 2, h.writerCount(), "a new writer is built for the new term")
	acq := h.lease.acquireCalls()
	assert.True(t, acq[len(acq)-1].newTerm)
}

// TestForceFreshResetRequiresLeaseAndKeepsFence is the plan T14 entry point for the proxy package: the supervisor
// carries forceFresh until Reset succeeds and runs Reset once per process, and the Redis state forces a fresh start
// only after a connection. The Reset script behaviour (keeps fence and fingerprint, fenced per batch, cursors first)
// is covered in redisstate: TestResetKeepsFenceAndFingerprintAndRemovesCursorsFirst,
// TestResetStopsWithErrFencedWhenLeaseMoves, TestResetWithStaleTokenDeletesNothing and the TestResetF* tests.
func TestForceFreshResetRequiresLeaseAndKeepsFence(t *testing.T) {
	t.Run("supervisor", TestWriterRoleResetPending)
	t.Run("only after connect", TestRedisStateForceFreshResetOnlyAfterConnect)
}

// countingWriter counts Start calls and the startup resync reads of the term's Writer.
type countingWriter struct {
	roleWriter
	owner  string
	token  int64
	starts atomic.Int32
	reads  *atomic.Int32
}

func (w *countingWriter) Start(src redisstate.Source) error {
	w.starts.Add(1)
	return w.roleWriter.Start(&countingSource{Source: src, reads: w.reads})
}

type countingSource struct {
	redisstate.Source
	reads *atomic.Int32
}

func (s *countingSource) Flags() ([]dtos.SplitDTO, int64, error) {
	s.reads.Add(1)
	return s.Source.Flags()
}

// twoSupervisorNode is one proxy of TestTwoSupervisorsOneRedisSingleWriter: a real supervisor on the real clock and the real lease client.
type twoSupervisorNode struct {
	sup     *redisStateSupervisor
	mu      sync.Mutex
	writers []*countingWriter
	reads   atomic.Int32
}

func (n *twoSupervisorNode) lastWriter() *countingWriter {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.writers) == 0 {
		return nil
	}
	return n.writers[len(n.writers)-1]
}

func (n *twoSupervisorNode) isWriter() bool { return n.sup.writer.Load() != nil }

func newTwoSupervisorNode(t *testing.T, cfg *pconf.Main) *twoSupervisorNode {
	t.Helper()
	logger, splits, rbs, segments := newRedisStateTestStorages(t)
	opts, err := common.ParseRedisOptions(&cfg.Storage.Redis.Connection)
	require.NoError(t, err)
	lease, err := redisstate.NewLeaseClient(opts)
	require.NoError(t, err)

	n := &twoSupervisorNode{}
	factory := productionWriterFactory(logger)
	n.sup = newRedisStateSupervisor(cfg, logger, splits, rbs, segments, newSourceGate(true), roleDeps{
		clock:     realClock{},
		lease:     lease,
		newClient: func() (*redis.PrefixedRedisClient, error) { return newRedisStateClient(opts, logger) },
		newWriter: func(c *redis.PrefixedRedisClient, k redisstate.Keys, fp string, o redisstate.Options) (roleWriter, error) {
			w, err := factory(c, k, fp, o)
			if err != nil {
				return nil, err
			}
			cw := &countingWriter{roleWriter: w, owner: o.Owner, token: o.Token, reads: &n.reads}
			n.mu.Lock()
			n.writers = append(n.writers, cw)
			n.mu.Unlock()
			return cw, nil
		},
	})
	go n.sup.run()
	t.Cleanup(n.sup.Stop)
	return n
}

// TestTwoSupervisorsOneRedisSingleWriter is the in-process stand-in for two proxies on one Redis: two supervisors with
// distinct owners, the real lease client and the real clock share one namespace with a 300 ms lease. It is not a
// multi-process or Sentinel run.
func TestTwoSupervisorsOneRedisSingleWriter(t *testing.T) {
	startRequireRedis(t)
	cfg := redisStateTestConfig(t)
	cfg.Storage.Redis.LeaseTtlMs = 300
	cfg.Storage.Redis.Connection.Prefix = "t-twosup-" + strings.ReplaceAll(t.Name(), "/", "-")
	opts, err := common.ParseRedisOptions(&cfg.Storage.Redis.Connection)
	require.NoError(t, err)

	admin := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	t.Cleanup(func() { _ = admin.Close() })
	ctx := context.Background()
	purge := func() {
		iter := admin.Scan(ctx, 0, cfg.Storage.Redis.Connection.Prefix+"*", 100).Iterator()
		for iter.Next(ctx) {
			admin.Del(ctx, iter.Val())
		}
	}
	purge()
	t.Cleanup(purge)

	client, err := newRedisStateClient(opts, logging.NewLogger(nil))
	require.NoError(t, err)
	keys, err := redisstate.NewKeys(client.Prefix(), cfg.Apikey, cfg.Storage.Redis.Namespace)
	require.NoError(t, err)
	fence := func() int64 {
		v, err := admin.HGet(ctx, keys.Meta(), redisstate.FenceField).Int64()
		if err != nil {
			return 0
		}
		return v
	}

	a := newTwoSupervisorNode(t, cfg)
	b := newTwoSupervisorNode(t, cfg)
	require.NotEqual(t, a.sup.owner, b.sup.owner)

	within := func(d time.Duration, cond func() bool, msg string) {
		t.Helper()
		deadline := time.Now().Add(d)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for: %s (a: %q b: %q)", msg, a.sup.Status().LastError, b.sup.Status().LastError)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	within(5*time.Second, func() bool { return a.isWriter() != b.isWriter() }, "exactly one supervisor holds the lease")
	holder, other := a, b
	if b.isWriter() {
		holder, other = b, a
	}
	within(2*time.Second, func() bool { return other.sup.Status().LastError == "not writer" }, "the other supervisor reports not writer")
	assert.False(t, other.isWriter())
	assert.Nil(t, other.lastWriter(), "no Writer is built while the lease is held elsewhere")
	holderWriter := holder.lastWriter()
	require.NotNil(t, holderWriter)
	fenceBefore := fence()
	require.Equal(t, holderWriter.token, fenceBefore)
	within(2*time.Second, func() bool { return holder.reads.Load() == 1 }, "the holder's startup resync")

	// Stay stable for several renew intervals: still one writer, same token.
	time.Sleep(700 * time.Millisecond)
	assert.True(t, holder.isWriter() && !other.isWriter(), "one writer across several renew intervals")
	assert.Equal(t, fenceBefore, fence())
	assert.Equal(t, int32(1), holder.reads.Load(), "no resync without a takeover")

	// Stop the holder: the lease is released and the other takes over.
	stopped := time.Now()
	holder.sup.Stop()
	within(300*time.Millisecond+100*time.Millisecond+2*time.Second, other.isWriter, "takeover by the other supervisor")
	t.Logf("takeover took %s", time.Since(stopped))
	newWriter := other.lastWriter()
	require.NotNil(t, newWriter)
	assert.Greater(t, newWriter.token, holderWriter.token, "the fence strictly increases")
	assert.Equal(t, newWriter.token, fence())
	within(2*time.Second, func() bool { return other.reads.Load() == 1 }, "the takeover resync")
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, int32(1), other.reads.Load(), "exactly one resync for the takeover term")
	assert.Equal(t, int32(1), newWriter.starts.Load())

	// A write with the old token is refused and changes nothing.
	before, err := admin.HGetAll(ctx, keys.Meta()).Result()
	require.NoError(t, err)
	staleOpts := redisstate.DefaultOptions()
	staleOpts.Owner = holderWriter.owner
	staleOpts.Token = holderWriter.token
	fp := util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
	stale, err := redisstate.NewWriter(client, keys, fp, staleOpts, logging.NewLogger(nil))
	require.NoError(t, err)
	err = stale.Reset()
	require.Error(t, err)
	assert.ErrorIs(t, err, redisstate.ErrFenced)
	after, err := admin.HGetAll(ctx, keys.Meta()).Result()
	require.NoError(t, err)
	assert.Equal(t, before, after, "the stale token changed nothing")
	assert.Equal(t, newWriter.owner, admin.Get(ctx, keys.Lease()).Val(), "the lease still belongs to the new holder")
}
