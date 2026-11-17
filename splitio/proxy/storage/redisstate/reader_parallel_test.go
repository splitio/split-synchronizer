package redisstate

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
