package redisstate

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/splitio/go-split-commons/v10/dtos"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fsFenceMover wraps a real client. The first Eval of script moves the lease to another owner before the call goes on,
// so the real script refuses that call. When hold is set, the first call signals entered and waits for hold to close
// first, so the test can queue changes while the write is in flight.
type fsFenceMover struct {
	Client
	admin   *goredis.Client
	lease   string
	script  string
	hold    chan struct{}
	entered chan struct{}
	mu      sync.Mutex
	calls   map[string]int
	moved   bool
}

func (c *fsFenceMover) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	c.calls[script]++
	move := script == c.script && !c.moved
	c.moved = c.moved || move
	c.mu.Unlock()
	if move {
		if c.hold != nil {
			close(c.entered)
			<-c.hold
		}
		_ = c.admin.Set(context.Background(), c.lease, "other", 0).Err()
	}
	return c.Client.Eval(script, keys, args...)
}

// count returns the Eval calls made for one script
func (c *fsFenceMover) count(script string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[script]
}

// total returns the Eval calls made for every script
func (c *fsFenceMover) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	sum := 0
	for _, n := range c.calls {
		sum += n
	}
	return sum
}

// fsNewWriter builds a writer on a real redis with the lease owned by testOwner. The first Eval of script moves the
// lease away (see fsFenceMover), so the writer is fenced at that call.
func fsNewWriter(t *testing.T, srv *ftServer, prefix, script string, hold, entered chan struct{}) (*Writer, *fsFenceMover, *wtLogger) {
	t.Helper()
	log := &wtLogger{}
	client, err := commonsredis.NewRedisClient(srv.cfg(prefix), log)
	require.NoError(t, err)
	keys, err := NewKeys(client.Prefix(), "ft-key", "")
	require.NoError(t, err)
	ftSeedLease(t, srv, keys)
	mover := &fsFenceMover{Client: client, admin: srv.admin, lease: keys.Lease(), script: script, hold: hold, entered: entered, calls: map[string]int{}}
	w, err := NewWriter(mover, keys, wtFingerprint, ftOpts(), log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return w, mover, log
}

// fsWaitDone waits until the worker goroutine has exited
func fsWaitDone(t *testing.T, w *Writer) {
	t.Helper()
	select {
	case <-w.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the worker did not exit after the fence")
	}
}

// fsAssertOneFenceError checks that the fence produced exactly one Error line and no line that says a pass reruns
func fsAssertOneFenceError(t *testing.T, errs []string) {
	t.Helper()
	assert.Len(t, errs, 1, "exactly one Error line for the fence, got %q", errs)
	for _, e := range errs {
		assert.Contains(t, e, "lease was lost")
		assert.NotContains(t, e, "resync failed, it reruns")
		assert.NotContains(t, e, "stopped early, it reruns")
	}
}

// (1) the lease moves while a resync runs: the flag resync write is refused, the worker is fenced and does not rerun
func TestFencedStateResyncStopsWhenLeaseMoves(t *testing.T) {
	srv := ftStartRedis(t)
	w, mover, _ := fsNewWriter(t, srv, "fsresync", FlagScript, nil, nil)
	src := &ftSource{flags: []dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE")}, flagsCn: 1, segs: []SegmentState{ftSeg("s1", 0, 3, 1)}}
	require.NoError(t, w.Start(src))
	fsWaitDone(t, w)

	assert.Equal(t, 1, mover.count(FlagScript), "the refused flag resync write is the only write")
	assert.Equal(t, int64(1), src.flagCall.Load(), "the source is read once, the resync does not rerun")
	assert.Zero(t, src.passes.Load(), "the segment pass never starts after the fence")
	assert.Zero(t, mover.count(SegmentScript), "no segment write after the fence")
	assert.False(t, w.Status().Healthy)
}

// (2) state after enterFenced: counters, LastError, worker exit, refusals, Stop
func TestFencedStateStatusCountersAndStop(t *testing.T) {
	srv := ftStartRedis(t)
	hold := make(chan struct{})
	entered := make(chan struct{})
	w, mover, _ := fsNewWriter(t, srv, "fsstate", FlagScript, hold, entered)
	require.NoError(t, w.Start(nil))

	w.PutFlags([]dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE")}, nil, 1)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the flag write never reached Redis")
	}
	w.PutSegment("s1", []string{"a"}, nil, 1)
	w.PutSegment("s2", []string{"b"}, nil, 1)
	w.PutSegment("s3", []string{"c"}, nil, 1)
	require.Eventually(t, func() bool { return w.Status().PendingChanges == 4 }, 10*time.Second, 10*time.Millisecond, "one in flight and three queued")
	pending := w.Status().PendingChanges
	close(hold)
	fsWaitDone(t, w)

	s := w.Status()
	assert.False(t, s.Healthy, "Healthy is false after the fence")
	assert.Contains(t, s.LastError, FencedMarker)
	assert.Equal(t, int64(pending), s.DroppedChanges, "the in-flight change and the queued changes are dropped")
	assert.Zero(t, s.PendingChanges, "the queue is cleared by the fence")
	assert.Zero(t, s.RefusedChanges, "the fence itself is not a refusal")

	evalsAtFence := mover.total()
	w.PutFlags([]dtos.SplitDTO{wtFlag("f2", 2, "ACTIVE")}, nil, 2)
	w.PutSegment("s4", []string{"d"}, nil, 2)
	s = w.Status()
	assert.Equal(t, int64(2), s.RefusedChanges, "changes after the fence are refused, not queued")
	assert.Zero(t, s.PendingChanges, "nothing is queued after the fence")
	assert.Equal(t, int64(pending), s.DroppedChanges, "refused changes are not counted as dropped")
	assert.Equal(t, evalsAtFence, mover.total(), "no Redis call after the fence")

	stopped := make(chan struct{})
	go func() {
		w.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung after the fence")
	}
	assert.Equal(t, int64(pending), w.Status().DroppedChanges, "Stop after the fence drops nothing more")
}

// (3) F-4: a fenced resync or a fenced startup purge logs exactly one Error line for the fence
func TestFencedStateLogsOneErrorForTheFence(t *testing.T) {
	t.Run("resync", func(t *testing.T) {
		srv := ftStartRedis(t)
		w, _, log := fsNewWriter(t, srv, "fserrres", FlagScript, nil, nil)
		src := &ftSource{flags: []dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE")}, flagsCn: 1}
		require.NoError(t, w.Start(src))
		fsWaitDone(t, w)
		fsAssertOneFenceError(t, log.ftErrors())
	})
	t.Run("startup purge", func(t *testing.T) {
		srv := ftStartRedis(t)
		w, mover, log := fsNewWriter(t, srv, "fserrpurge", PurgeScript, nil, nil)
		require.NoError(t, w.Start(nil))
		fsWaitDone(t, w)
		require.Positive(t, mover.count(PurgeScript), "the startup purge reached Redis")
		fsAssertOneFenceError(t, log.ftErrors())
	})
}

// (4) F-5: after the fence RequestResync does not schedule a resync
func TestFencedStateRequestResyncIsIgnored(t *testing.T) {
	srv := ftStartRedis(t)
	w, _, _ := fsNewWriter(t, srv, "fsreq", FlagScript, nil, nil)
	src := &ftSource{flags: []dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE")}, flagsCn: 1}
	require.NoError(t, w.Start(src))
	fsWaitDone(t, w)

	w.RequestResync()

	w.mtx.Lock()
	need, since := w.needResync, w.resyncSince
	w.mtx.Unlock()
	assert.False(t, need, "RequestResync must not set needResync on a fenced writer")
	assert.True(t, since.IsZero(), "no resync is pending after the fence")
	assert.Equal(t, int64(1), src.flagCall.Load(), "no resync ran after the fence")
}

// (5) F-6: Abort logs at Info when it drops nothing and at Error when it drops changes
func TestFencedStateAbortLogLevel(t *testing.T) {
	keys := wtKeys(t)

	t.Run("nothing pending logs info", func(t *testing.T) {
		log := &wtLogger{}
		w, err := NewWriter(&wtClient{}, keys, wtFingerprint, wtOptions(), log)
		require.NoError(t, err)
		w.Abort()
		assert.Zero(t, log.errorCount(), "an abort that drops nothing is not an error")
		assert.NotEmpty(t, log.infoText(), "the abort is logged at Info")
	})

	t.Run("pending logs error", func(t *testing.T) {
		log := &wtLogger{}
		w, err := NewWriter(&wtClient{}, keys, wtFingerprint, wtOptions(), log)
		require.NoError(t, err)
		w.PutSegment("s1", []string{"a"}, nil, 1)
		require.Equal(t, 1, w.Status().PendingChanges)
		w.Abort()
		assert.Equal(t, 1, log.errorCount(), "an abort that drops changes is logged at Error")
		assert.Equal(t, int64(1), w.Status().DroppedChanges)
	})
}

// (6) F-9: the token must be a positive value that fits the 15 digits the fencing scripts accept
func TestFencedStateTokenBounds(t *testing.T) {
	keys := wtKeys(t)
	cases := []struct {
		token int64
		ok    bool
	}{
		{999999999999999, true},
		{1000000000000000, false},
		{0, false},
		{-1, false},
	}
	for _, tc := range cases {
		t.Run("token "+strconv.FormatInt(tc.token, 10), func(t *testing.T) {
			o := wtOptions()
			o.Token = tc.token
			_, err := NewWriter(&wtClient{}, keys, wtFingerprint, o, &wtLogger{})
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrInvalidOptions)
		})
	}
}
