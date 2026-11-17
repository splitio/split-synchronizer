package redisstate

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	cconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedClient serializes calls into rdFake, which is not safe for concurrent use. before runs first, outside the
// lock, so it may sleep, count calls, or return an error that replaces the call's result.
type lockedClient struct {
	mu     sync.Mutex
	inner  *rdFake
	before func(op, key string) error
}

func (c *lockedClient) enter(op, key string) error {
	if c.before == nil {
		return nil
	}
	return c.before(op, key)
}

func (c *lockedClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if err := c.enter("HGetAll", key); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.HGetAll(ctx, key)
}

func (c *lockedClient) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if err := c.enter("ZScan", key); err != nil {
		return nil, 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.ZScan(ctx, key, cursor, count)
}

func (c *lockedClient) ZCard(ctx context.Context, key string) (int64, error) {
	if err := c.enter("ZCard", key); err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.ZCard(ctx, key)
}

func (c *lockedClient) HGet(ctx context.Context, key, field string) (string, bool, error) {
	if err := c.enter("HGet", key); err != nil {
		return "", false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inner.HGet(ctx, key, field)
}

// churnCursor answers HGet for one cursor field with a value that changes on every call, so that segment stays torn.
// Other fields go to the locked client.
type churnCursor struct {
	*lockedClient
	field string
	calls atomic.Int64
}

func (c *churnCursor) HGet(ctx context.Context, key, field string) (string, bool, error) {
	if field != c.field {
		return c.lockedClient.HGet(ctx, key, field)
	}
	return strconv.Itoa(100 + int(c.calls.Add(1))), true, nil
}

// seedEight is seedValid with flag f1 referencing s1..s8, so all eight segments are restored. Segment cursors are
// 41..48 in order.
func (s *rdFixture) seedEight() []string {
	s.seedValid()
	k := s.keys
	names := make([]string, 0, 8)
	meta := map[string]string{}
	for i := 1; i <= 8; i++ {
		name := "s" + strconv.Itoa(i)
		names = append(names, name)
		meta[k.SegmentCursorField(name)] = strconv.Itoa(40 + i)
	}
	s.hash(k.Meta(), meta)
	s.hash(k.FlagData(), map[string]string{"f1": rdFlagJSON(s.t, "f1", statusActive, 10, names...)})
	for i := 3; i <= 8; i++ {
		s.scores(k.SegmentActive(names[i-1]), map[string]int64{"k" + names[i-1]: int64(100 + i)})
	}
	return names
}

// activeKeys maps each segment active key to its 1-based position in names.
func (s *rdFixture) activeKeys(names []string) map[string]int {
	out := map[string]int{}
	for i, name := range names {
		out[s.keys.SegmentActive(name)] = i + 1
	}
	return out
}

func (s *rdFixture) parallelReader(c ReadClient, n int) *Reader {
	s.t.Helper()
	r, err := NewReader(c, s.keys, rdFp, withSegmentParallelism(n), withRetryPause(0))
	require.NoError(s.t, err)
	return r
}

func TestReader_ParallelSegmentsDeliveredInOrder(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if i, ok := idx[key]; ok && op == "ZScan" {
			// Earlier segments sleep longer, so they finish after later ones.
			time.Sleep(time.Duration(9-i) * 4 * time.Millisecond)
		}
		return nil
	}}
	state, segs, err := rdRead(context.Background(), s.parallelReader(lc, 4))
	require.NoError(t, err)

	got := make([]string, 0, len(segs))
	for _, seg := range segs {
		got = append(got, seg.Name)
	}
	assert.Equal(t, names, got)
	assert.Equal(t, 8, state.SegmentCount)
	assert.Empty(t, state.SkippedSegments)
	for i, name := range names {
		cursor, ok := state.SegmentCursors[name]
		assert.True(t, ok, "cursor for %s", name)
		assert.Equal(t, int64(41+i), cursor, "cursor for %s", name)
	}
}

func TestReader_ParallelismBoundsInFlightSegments(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	active := s.activeKeys(names)
	var inFlight, maxSeen atomic.Int64
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if _, ok := active[key]; !ok || op != "ZScan" {
			return nil
		}
		now := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			m := maxSeen.Load()
			if now <= m || maxSeen.CompareAndSwap(m, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		return nil
	}}
	_, segs, err := rdRead(context.Background(), s.parallelReader(lc, 3))
	require.NoError(t, err)
	assert.Len(t, segs, 8)
	assert.LessOrEqual(t, maxSeen.Load(), int64(3))
	assert.Greater(t, maxSeen.Load(), int64(1))
}

func TestReader_ParallelSegmentErrorCancelsAndStops(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	failing := s.keys.SegmentActive("s5")
	var inFlight atomic.Int64
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if op != "ZScan" {
			return nil
		}
		inFlight.Add(1)
		defer inFlight.Add(-1)
		if key == failing {
			return ErrUnreachable
		}
		if _, ok := idx[key]; ok {
			time.Sleep(3 * time.Millisecond)
		}
		return nil
	}}
	var got []string
	_, err := s.parallelReader(lc, 4).Read(context.Background(), func(seg SegmentState) error {
		got = append(got, seg.Name)
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.Equal(t, names[:4], got)
	assert.Equal(t, int64(0), inFlight.Load(), "a segment read was still running after Read returned")
}

func TestReader_ParallelTornSegmentIsSkippedOthersRestored(t *testing.T) {
	s := newRdFixture(t)
	s.seedEight()
	torn := &churnCursor{
		lockedClient: &lockedClient{inner: s.fake},
		field:        s.keys.SegmentCursorField("s3"),
	}
	state, segs, err := rdRead(context.Background(), s.parallelReader(torn, 4))
	require.NoError(t, err)
	assert.Equal(t, []string{"s3"}, state.SkippedSegments)
	assert.Equal(t, 7, state.SegmentCount)
	assert.Len(t, segs, 7)
	assert.Equal(t, int64(tornReadAttempts), torn.calls.Load())
}

func TestReader_ParallelismBelowOneIsOne(t *testing.T) {
	s := newRdFixture(t)
	for _, n := range []int{0, -3} {
		r, err := NewReader(s.fake, s.keys, rdFp, withSegmentParallelism(n))
		require.NoError(t, err)
		assert.Equal(t, 1, r.parallelism, "parallelism for option %d", n)
	}
}

func TestReader_SinkErrorStopsDelivery(t *testing.T) {
	s := newRdFixture(t)
	s.seedEight()
	errSink := errors.New("sink refused segment")
	r := s.parallelReader(&lockedClient{inner: s.fake}, 4)
	calls := 0
	_, err := r.Read(context.Background(), func(SegmentState) error {
		calls++
		if calls == 3 {
			return errSink
		}
		return nil
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSink)
	assert.Equal(t, 3, calls)
}

// seedN is seedEight for n segments. It returns the names in the order the reader delivers them (sorted by name).
func (s *rdFixture) seedN(n int) []string {
	s.seedValid()
	k := s.keys
	names := make([]string, 0, n)
	meta := map[string]string{}
	for i := 1; i <= n; i++ {
		name := "s" + strconv.Itoa(i)
		names = append(names, name)
		meta[k.SegmentCursorField(name)] = strconv.Itoa(40 + i)
		if i >= 3 {
			s.scores(k.SegmentActive(name), map[string]int64{"k" + name: int64(100 + i)})
		}
	}
	s.hash(k.Meta(), meta)
	s.hash(k.FlagData(), map[string]string{"f1": rdFlagJSON(s.t, "f1", statusActive, 10, names...)})
	sort.Strings(names)
	return names
}

// ctxClient is a lockedClient whose ZScan hook also gets the call context.
type ctxClient struct {
	*lockedClient
	zscan func(ctx context.Context, key string) error
}

func (c *ctxClient) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if c.zscan != nil {
		if err := c.zscan(ctx, key); err != nil {
			return nil, 0, err
		}
	}
	return c.lockedClient.ZScan(ctx, key, cursor, count)
}

// recSink records delivered segments and how many Segment calls overlapped.
type recSink struct {
	mu       sync.Mutex
	names    []string
	inFlight atomic.Int64
	maxSeen  atomic.Int64
	failAt   int
	err      error
	onFail   func()
}

func (r *recSink) Entities([]dtos.SplitDTO, []dtos.RuleBasedSegmentDTO) error { return nil }

func (r *recSink) Segment(seg SegmentState) error {
	now := r.inFlight.Add(1)
	defer r.inFlight.Add(-1)
	for {
		m := r.maxSeen.Load()
		if now <= m || r.maxSeen.CompareAndSwap(m, now) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, seg.Name)
	if r.failAt > 0 && len(r.names) == r.failAt {
		if r.onFail != nil {
			r.onFail()
		}
		return r.err
	}
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReader_ParallelSinkInOrderOneAtATime(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if i, ok := idx[key]; ok && op == "ZScan" {
			time.Sleep(time.Duration(9-i) * 4 * time.Millisecond)
		}
		return nil
	}}
	sink := &recSink{}
	state, err := s.parallelReader(lc, 4).ReadInto(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, names, sink.names)
	assert.LessOrEqual(t, sink.maxSeen.Load(), int64(1))
	assert.Equal(t, 8, state.SegmentCount)
	assert.Empty(t, state.Segments, "ReadInto must not retain payloads")
}

func TestReader_ParallelWorkersDoNotWriteBootState(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedN(12)
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if op == "ZScan" {
			time.Sleep(time.Millisecond)
		}
		return nil
	}}
	sink := &recSink{}
	state, err := s.parallelReader(lc, 4).ReadInto(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, names, sink.names)
	assert.Equal(t, len(names), state.SegmentCount)
	assert.Len(t, state.SegmentCursors, len(names))
	for _, n := range names {
		_, ok := state.SegmentCursors[n]
		assert.True(t, ok, "cursor for %s", n)
	}
	assert.Equal(t, int64(0), sink.inFlight.Load())
}

func TestReader_ParallelMemoryWindowReleasedOnDelivery(t *testing.T) {
	const p = 2
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	head := s.keys.SegmentActive("s1")
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var started sync.Map
	var startedN atomic.Int64
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if _, ok := idx[key]; !ok || op != "ZScan" {
			return nil
		}
		if _, dup := started.LoadOrStore(key, true); !dup {
			startedN.Add(1)
		}
		if key == head {
			select {
			case <-release:
			case <-time.After(2 * time.Second):
			}
		}
		return nil
	}}
	type result struct {
		state BootState
		err   error
	}
	done := make(chan result, 1)
	sink := &recSink{}
	go func() {
		st, err := s.parallelReader(lc, p).ReadInto(context.Background(), sink)
		done <- result{st, err}
	}()
	waitFor(t, "both initial segments to start", func() bool { return startedN.Load() >= p })
	time.Sleep(40 * time.Millisecond)
	assert.LessOrEqual(t, startedN.Load(), int64(p+1), "segments started while the head is blocked")
	assert.Empty(t, sink.names, "nothing may be delivered before the head")

	once.Do(func() { close(release) })
	select {
	case res := <-done:
		require.NoError(t, res.err)
		assert.Equal(t, 8, res.state.SegmentCount)
	case <-time.After(2 * time.Second):
		t.Fatal("read did not finish after the head was released")
	}
	assert.Equal(t, int64(8), startedN.Load())
	assert.Equal(t, names, sink.names)
}

func TestReader_ParallelBudgetErrorIsCorruptNotUnreachable(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		i, ok := idx[key]
		if !ok || op != "ZScan" || i < 3 || i > 6 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
		if i == 3 {
			return context.DeadlineExceeded
		}
		return nil
	}}
	r, err := NewReader(lc, s.keys, rdFp, withSegmentParallelism(4), withRetryPause(0), withRestoreBudget(60*time.Millisecond))
	require.NoError(t, err)
	state, err := r.ReadInto(context.Background(), &recSink{})
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.True(t, state.BudgetExhausted)
	assert.NotEqual(t, BootUnreachable, state.Status)
}

func TestReader_ParallelSinkErrorIsWrappedNotCorrupt(t *testing.T) {
	s := newRdFixture(t)
	s.seedEight()
	errSink := errors.New("sink refused segment")
	sink := &recSink{failAt: 3, err: errSink}
	state, err := s.parallelReader(&lockedClient{inner: s.fake}, 4).ReadInto(context.Background(), sink)
	require.Error(t, err)
	assert.ErrorIs(t, err, errSink)
	assert.NotErrorIs(t, err, ErrUnreachable)
	assert.NotErrorIs(t, err, errCorrupt)
	assert.Contains(t, err.Error(), "restoring segment s3")
	assert.NotEqual(t, BootCorrupt, state.Status)
	assert.NotEqual(t, BootUnreachable, state.Status)
	assert.Len(t, sink.names, 3)
}

func TestReader_ParallelParentCancelIsUnreachable(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if i, ok := idx[key]; ok && op == "ZScan" && i == 3 {
			cancel()
			return context.Canceled
		}
		return nil
	}}
	state, err := s.parallelReader(lc, 4).ReadInto(ctx, &recSink{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.Equal(t, BootUnreachable, state.Status)
}

func TestReader_ParallelHardErrorWinsOverSiblingCancel(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	var siblings atomic.Int64
	var seen sync.Map
	c := &ctxClient{lockedClient: &lockedClient{inner: s.fake}}
	c.zscan = func(ctx context.Context, key string) error {
		i, ok := idx[key]
		if !ok || i < 3 || i > 6 {
			return nil
		}
		if i == 3 {
			waitSiblings := time.Now().Add(2 * time.Second)
			for siblings.Load() < 3 && time.Now().Before(waitSiblings) {
				time.Sleep(time.Millisecond)
			}
			return errors.New("boom: storage failure")
		}
		if _, dup := seen.LoadOrStore(key, true); !dup {
			siblings.Add(1)
		}
		<-ctx.Done()
		return ctx.Err()
	}
	_, err := s.parallelReader(c, 4).ReadInto(context.Background(), &recSink{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom: storage failure")
	assert.NotContains(t, err.Error(), "context canceled")
	assert.GreaterOrEqual(t, siblings.Load(), int64(3), "siblings were not in flight")
}

func TestReader_ParallelSinkErrorBeatsLateCorruptSibling(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	errSink := errors.New("sink refused segment")
	c := &ctxClient{lockedClient: &lockedClient{inner: s.fake}}
	c.zscan = func(ctx context.Context, key string) error {
		if idx[key] != 2 {
			return nil
		}
		<-ctx.Done()
		return errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	sink := &recSink{failAt: 1, err: errSink}
	state, err := s.parallelReader(c, 2).ReadInto(context.Background(), sink)
	require.Error(t, err)
	assert.ErrorIs(t, err, errSink)
	assert.NotEqual(t, BootCorrupt, state.Status)
	assert.Empty(t, state.SkippedSegments)
}

// A corrupt read that completes after the context was cancelled must not become a skip: the segment was not really
// judged, so the read has to fail instead of reporting a valid state with the segment skipped.
func TestReader_ParallelCorruptAfterCancelIsNotSkipped(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started atomic.Bool
	c := &ctxClient{lockedClient: &lockedClient{inner: s.fake}}
	c.zscan = func(cctx context.Context, key string) error {
		if idx[key] != 2 {
			return nil
		}
		started.Store(true)
		<-cctx.Done()
		return errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	go func() {
		for !started.Load() {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	state, err := s.parallelReader(c, 2).ReadInto(ctx, &recSink{})
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.Empty(t, state.SkippedSegments)
}

func TestReader_ParallelNoClientCallAfterReturn(t *testing.T) {
	cases := []struct {
		name string
		run  func(s *rdFixture, c ReadClient, idx map[string]int) error
	}{
		{"success", func(s *rdFixture, c ReadClient, _ map[string]int) error {
			_, err := s.parallelReader(c, 4).ReadInto(context.Background(), &recSink{})
			return err
		}},
		{"sink error", func(s *rdFixture, c ReadClient, _ map[string]int) error {
			_, err := s.parallelReader(c, 4).ReadInto(context.Background(), &recSink{failAt: 2, err: errors.New("sink")})
			return err
		}},
		{"read error", func(s *rdFixture, c ReadClient, _ map[string]int) error {
			_, err := s.parallelReader(c, 4).Read(context.Background(), func(SegmentState) error { return nil })
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newRdFixture(t)
			names := s.seedEight()
			idx := s.activeKeys(names)
			baseline := runtime.NumGoroutine()
			var calls atomic.Int64
			var returned atomic.Bool
			var late atomic.Int64
			failing := s.keys.SegmentActive("s5")
			lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
				calls.Add(1)
				if returned.Load() {
					late.Add(1)
				}
				if _, ok := idx[key]; ok && op == "ZScan" {
					time.Sleep(2 * time.Millisecond)
					if tc.name == "read error" && key == failing {
						return ErrUnreachable
					}
				}
				return nil
			}}
			_ = tc.run(s, lc, idx)
			atReturn := calls.Load()
			returned.Store(true)
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, atReturn, calls.Load(), "client called after the read returned")
			assert.Equal(t, int64(0), late.Load())
			waitFor(t, "goroutines to return to baseline", func() bool { return runtime.NumGoroutine() <= baseline })
		})
	}
}

func TestReader_ParallelReadRetainsSegmentsInOrder(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	idx := s.activeKeys(names)
	lc := &lockedClient{inner: s.fake, before: func(op, key string) error {
		if i, ok := idx[key]; ok && op == "ZScan" {
			time.Sleep(time.Duration(9-i) * 4 * time.Millisecond)
		}
		return nil
	}}
	state, err := s.parallelReader(lc, 4).Read(context.Background(), nil)
	require.NoError(t, err)
	got := make([]string, 0, len(state.Segments))
	for _, seg := range state.Segments {
		got = append(got, seg.Name)
	}
	assert.Equal(t, names, got)
	assert.Equal(t, 8, state.SegmentCount)
}

func TestReader_ParallelTornSegmentReasonMentionsCursorChange(t *testing.T) {
	s := newRdFixture(t)
	names := s.seedEight()
	torn := &churnCursor{
		lockedClient: &lockedClient{inner: s.fake},
		field:        s.keys.SegmentCursorField("s6"),
	}
	state, err := s.parallelReader(torn, 4).ReadInto(context.Background(), &recSink{})
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"s6"}, state.SkippedSegments)
	assert.Contains(t, state.SkipReasons["s6"], "cursor changed while it was read")
	assert.Equal(t, 7, state.SegmentCount)
	for _, n := range names {
		_, ok := state.SegmentCursors[n]
		assert.Equal(t, n != "s6", ok, "cursor for %s", n)
	}
	assert.Equal(t, int64(tornReadAttempts), torn.calls.Load())
}

// The reader needs parallelism+1 pooled connections so every worker has one and the coordinator is never starved.
// The pool size is user configurable (redis pool option), so only the default is guaranteed.
func TestReader_ParallelDefaultBootPoolFitsParallelism(t *testing.T) {
	var cfg pconf.Main
	require.NoError(t, cconf.PopulateDefaults(&cfg))
	redisCfg, err := common.ParseRedisOptions(&cfg.Storage.Redis.Connection)
	require.NoError(t, err)
	opts, _, err := universalOptions(redisCfg)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, opts.PoolSize, defaultSegmentParallelism+1)
}
