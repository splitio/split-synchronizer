package redisstate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// f4bRedisErr is a redis error reply: it has the RedisError method that go-redis uses for server replies.
type f4bRedisErr string

func (e f4bRedisErr) Error() string { return string(e) }
func (f4bRedisErr) RedisError()     {}

// f4bNetErr is a net.Error with a fixed Timeout answer.
type f4bNetErr struct{ timeout bool }

func (e f4bNetErr) Error() string   { return "net error" }
func (e f4bNetErr) Timeout() bool   { return e.timeout }
func (e f4bNetErr) Temporary() bool { return false }

// F4-05: transient errors are matched by type first. Text matches stay as a fallback.
func TestIsTransientTyped(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"io.EOF wrapped", fmt.Errorf("read: %w", io.EOF), true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"ECONNRESET wrapped", fmt.Errorf("x: %w", syscall.ECONNRESET), true},
		{"ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"EPIPE", fmt.Errorf("x: %w", syscall.EPIPE), true},
		{"dial OpError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("no route")}, true},
		{"read OpError", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("x")}, true},
		{"write OpError", &net.OpError{Op: "write", Net: "tcp", Err: errors.New("x")}, true},
		{"net timeout", f4bNetErr{timeout: true}, true},
		{"net error without timeout", f4bNetErr{timeout: false}, false},
		{"context deadline is retried by callWithRetry, not here", context.DeadlineExceeded, false},
		{"LOADING reply", f4bRedisErr("LOADING Redis is loading the dataset"), true},
		{"BUSY reply", f4bRedisErr("BUSY Redis is busy running a script"), true},
		{"READONLY reply", f4bRedisErr("READONLY You can't write against a read only replica."), true},
		{"TRYAGAIN reply", f4bRedisErr("TRYAGAIN Multiple keys request during rehashing"), true},
		{"MASTERDOWN reply", f4bRedisErr("MASTERDOWN Link with MASTER is down"), true},
		{"CLUSTERDOWN reply", f4bRedisErr("CLUSTERDOWN The cluster is down"), true},
		{"MOVED reply", f4bRedisErr("MOVED 3999 127.0.0.1:6381"), true},
		{"ASK reply wrapped", fmt.Errorf("cmd: %w", f4bRedisErr("ASK 3999 127.0.0.1:6381")), true},
		{"pool timeout", errors.New("redis: connection pool timeout"), true},
		{"WRONGTYPE is not transient", f4bRedisErr("WRONGTYPE Operation against a key holding the wrong kind of value"), false},
		{"NOAUTH is not transient", f4bRedisErr("NOAUTH Authentication required."), false},
		{"NOPERM is not transient", f4bRedisErr("NOPERM this user has no permissions"), false},
		{"auth text without redis type is not transient", errors.New("NOAUTH Authentication required."), false},
		{"other redis error is not transient", f4bRedisErr("ERR syntax error"), false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isTransient(tc.err))
		})
	}
}

// F4-05: the retry pause grows with the attempt, so the waits are pause, then 2 * pause.
func TestReadCallRetryPauseGrowsPerAttempt(t *testing.T) {
	const pause = 20 * time.Millisecond
	calls := 0
	start := time.Now()
	_, err := readCall(context.Background(), pause, "meta", func(context.Context) (int, error) {
		calls++
		return 0, errors.New("connection reset by peer")
	})
	require.ErrorIs(t, err, ErrUnreachable)
	assert.Equal(t, readAttempts, calls)
	assert.GreaterOrEqual(t, time.Since(start), 3*pause, "pauses of 1x and 2x the base pause")
}

// F4-06: once the restore budget has expired, any error is the budget running out, not an unreachable redis.
func TestReadCallBudgetExpiryWithNonTimeoutErrorIsBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	_, err := readCall(ctx, 0, "meta", func(context.Context) (int, error) {
		return 0, errors.New("redis: connection pool timeout")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBudgetExhausted)
	assert.ErrorIs(t, err, errCorrupt)
	assert.NotErrorIs(t, err, ErrUnreachable)
}

// F4-06: a cancelled caller stays unreachable, the budget does not apply.
func TestReadCallCancelledStaysUnreachable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readCall(ctx, 0, "meta", func(context.Context) (int, error) {
		return 0, errors.New("redis: connection pool timeout")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.NotErrorIs(t, err, ErrBudgetExhausted)
}

// F4-02: whether a flag or rule-based segment group is corrupt depends only on its own entities and cursor. Segment
// names in meta never change the verdict.
func TestReadCursorlessOutcomeAgreesWithAndWithoutSegments(t *testing.T) {
	shapes := []struct {
		name string
		seed func(s *rdFixture)
		want BootStatus
	}{
		{
			name: "a: no cursors and no entities",
			seed: func(s *rdFixture) {},
			want: BootAbsent,
		},
		{
			name: "c: rule-based segment cursor only",
			seed: func(s *rdFixture) {
				s.hash(s.keys.Meta(), map[string]string{s.keys.RuleBasedSegmentCursorField(): "20"})
			},
			want: BootValid,
		},
		{
			name: "d: flags stored without a flag cursor",
			seed: func(s *rdFixture) {
				s.hash(s.keys.FlagData(), map[string]string{"f1": rdFlagJSON(s.t, "f1", statusActive, 9)})
				s.scores(s.keys.FlagIndex(), map[string]int64{"f1": 9})
			},
			want: BootCorrupt,
		},
		{
			name: "e: rule-based segments stored without a rule-based cursor",
			seed: func(s *rdFixture) {
				s.hash(s.keys.Meta(), map[string]string{s.keys.FlagCursorField(): "10"})
				s.hash(s.keys.RuleBasedSegmentData(), map[string]string{"r1": rdRbsJSON(s.t, "r1", statusActive, 20)})
				s.scores(s.keys.RuleBasedSegmentIndex(), map[string]int64{"r1": 20})
			},
			want: BootCorrupt,
		},
	}
	for _, shape := range shapes {
		for _, withSegment := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s segment=%v", shape.name, withSegment), func(t *testing.T) {
				s := newRdFixture(t)
				s.hash(s.keys.Meta(), map[string]string{FingerprintField: rdFp})
				shape.seed(s)
				if withSegment {
					s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "30"})
					s.scores(s.keys.SegmentActive("s1"), map[string]int64{"k1": 30})
				}

				state, _, err := rdRead(context.Background(), s.reader())
				require.NoError(t, err)
				assert.Equal(t, shape.want, state.Status)
			})
		}
	}
}
