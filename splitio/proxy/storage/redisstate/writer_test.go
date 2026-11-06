package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wtFingerprint = "fp-test"

type wtCall struct {
	script string
	keys   []string
	args   []interface{}
}

// wtClient is a fake Client that records every call. fail is consulted for each Eval (n is the 1-based call number)
// and may block; it is set before Start, so the worker reads it without a lock.
type wtClient struct {
	mu    sync.Mutex
	calls []wtCall
	hgets []string
	meta  map[string]string
	fail  func(n int, c wtCall) error
}

func (c *wtClient) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	call := wtCall{script: script, keys: keys, args: args}
	c.calls = append(c.calls, call)
	n := len(c.calls)
	c.mu.Unlock()
	if c.fail != nil {
		return c.fail(n, call)
	}
	return nil
}

func (c *wtClient) HGetAll(key string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hgets = append(c.hgets, key)
	out := map[string]string{}
	for k, v := range c.meta {
		out[k] = v
	}
	return out, nil
}

func (c *wtClient) Prefix() string { return "" }

func (c *wtClient) evals() []wtCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]wtCall(nil), c.calls...)
}

func (c *wtClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func (c *wtClient) hgetKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.hgets...)
}

// wtLogger counts Error lines and Info lines
type wtLogger struct {
	mu     sync.Mutex
	errors []string
	infos  int
}

func (l *wtLogger) Error(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprint(msg...))
}

func (l *wtLogger) Warning(msg ...interface{}) {}

func (l *wtLogger) Info(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos++
}

func (l *wtLogger) Debug(msg ...interface{})   {}
func (l *wtLogger) Verbose(msg ...interface{}) {}

func (l *wtLogger) errorCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.errors)
}

// wtClock is a fake clock that only moves when the test or the fake wait moves it
type wtClock struct {
	mu sync.Mutex
	t  time.Time
}

func newWtClock() *wtClock {
	return &wtClock{t: time.Unix(1700000000, 0)}
}

func (c *wtClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *wtClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// wtSource is a fake Source. Fields are only touched by the worker, or by the test after Stop.
type wtSource struct {
	flags      []dtos.SplitDTO
	flagsCn    int64
	flagsFails int
	flagsCalls int
	onFlags    func()
	rbs        []dtos.RuleBasedSegmentDTO
	rbsCn      int64
	segs       []SegmentState
	segsFails  int
}

func (s *wtSource) Flags() ([]dtos.SplitDTO, int64, error) {
	s.flagsCalls++
	if s.onFlags != nil {
		s.onFlags()
	}
	if s.flagsFails > 0 {
		s.flagsFails--
		return nil, 0, errors.New("local state busy")
	}
	return s.flags, s.flagsCn, nil
}

func (s *wtSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	return s.rbs, s.rbsCn, nil
}

func (s *wtSource) Segments(visit func(SegmentState) error) error {
	if s.segsFails > 0 {
		s.segsFails--
		return errors.New("local state busy")
	}
	for _, seg := range s.segs {
		if err := visit(seg); err != nil {
			return err
		}
	}
	return nil
}

// wtEnv bundles a Writer with its fakes. Waits are instant unless a test replaces w.wait.
type wtEnv struct {
	t      *testing.T
	opts   Options
	client *wtClient
	clock  *wtClock
	log    *wtLogger
	w      *Writer
	once   sync.Once
}

func wtOptions() Options {
	o := DefaultOptions()
	o.MaxAttempts = 3
	o.BackoffMaxWait = time.Hour
	return o
}

func wtKeys(t *testing.T) Keys {
	t.Helper()
	k, err := NewKeys("t-"+t.Name(), "apikey-test", "")
	require.NoError(t, err)
	return k
}

func newWtEnv(t *testing.T, opts Options) *wtEnv {
	t.Helper()
	e := &wtEnv{t: t, opts: opts, client: &wtClient{}, clock: newWtClock(), log: &wtLogger{}}
	w, err := NewWriter(e.client, wtKeys(t), wtFingerprint, opts, e.log)
	require.NoError(t, err)
	w.now = e.clock.Now
	w.wait = func(d time.Duration) bool {
		e.clock.Add(d)
		return !w.stopped()
	}
	e.w = w
	t.Cleanup(e.stop)
	return e
}

// stop stops the writer once; it is safe to call from the test body and from Cleanup
func (e *wtEnv) stop() {
	e.once.Do(func() { e.w.Stop() })
}

// wtWaitUntil polls cond for up to 5 seconds
func wtWaitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func wtFlag(name string, cn int64, status string) dtos.SplitDTO {
	return dtos.SplitDTO{Name: name, ChangeNumber: cn, Status: status}
}

func wtFlags(names ...string) []dtos.SplitDTO {
	out := make([]dtos.SplitDTO, 0, len(names))
	for _, n := range names {
		out = append(out, wtFlag(n, 1, "ACTIVE"))
	}
	return out
}

func wtKeysOf(n int, prefix string) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("%s%d", prefix, i))
	}
	return out
}

type wtEnt struct {
	name    string
	cn      int64
	removed string
	json    string
}

// wtEntities decodes the entity part of a flags or rule-based segments script call
func wtEntities(args []interface{}) []wtEnt {
	n := args[5].(int)
	out := []wtEnt{}
	for i := 0; i < n; i++ {
		b := 6 + 4*i
		out = append(out, wtEnt{name: args[b].(string), cn: args[b+1].(int64), removed: args[b+2].(string), json: args[b+3].(string)})
	}
	return out
}

func wtFlagCursor(args []interface{}) int64 {
	return args[4].(int64)
}

type wtSegKey struct {
	key string
	cn  int64
}

// wtSegArgs decodes a segment script call: name, cursor, adds, removes
func wtSegArgs(args []interface{}) (string, int64, []wtSegKey, []wtSegKey) {
	name := args[3].(string)
	cursor := args[5].(int64)
	nAdd := args[6].(int)
	nRem := args[7].(int)
	var adds, rems []wtSegKey
	for i := 0; i < nAdd; i++ {
		b := 8 + 2*i
		adds = append(adds, wtSegKey{key: args[b].(string), cn: args[b+1].(int64)})
	}
	for i := 0; i < nRem; i++ {
		b := 8 + 2*nAdd + 2*i
		rems = append(rems, wtSegKey{key: args[b].(string), cn: args[b+1].(int64)})
	}
	return name, cursor, adds, rems
}

func TestWriterRejectsInvalidOptions(t *testing.T) {
	keys := wtKeys(t)
	cases := map[string]func(o *Options){
		"retention":    func(o *Options) { o.Retention = 0 },
		"max attempts": func(o *Options) { o.MaxAttempts = 0 },
		"backoff":      func(o *Options) { o.BackoffMaxWait = 0 },
		"pending":      func(o *Options) { o.MaxPendingChanges = 0 },
		"chunk size":   func(o *Options) { o.ChunkSize = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			o := wtOptions()
			mutate(&o)
			_, err := NewWriter(&wtClient{}, keys, wtFingerprint, o, &wtLogger{})
			assert.ErrorIs(t, err, ErrInvalidOptions)
		})
	}
	_, err := NewWriter(&wtClient{}, keys, wtFingerprint, DefaultOptions(), &wtLogger{})
	assert.NoError(t, err)
}

func TestWriterSuccess(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("f1", 5, "ACTIVE")}, nil, 5)
	e.w.Start(nil)
	wtWaitUntil(t, "the flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, 1)
	assert.Equal(t, FlagScript, calls[0].script)
	assert.Equal(t, e.w.keys.FlagKeys(), calls[0].keys)
	args := calls[0].args
	assert.Equal(t, wtFingerprint, args[0])
	assert.Equal(t, e.clock.Now().UnixMilli(), args[1])
	assert.Equal(t, e.opts.Retention.Milliseconds(), args[2])
	assert.Equal(t, e.w.keys.FlagCursorField(), args[3])
	assert.Equal(t, int64(5), wtFlagCursor(args))
	ents := wtEntities(args)
	require.Len(t, ents, 1)
	assert.Equal(t, "f1", ents[0].name)
	assert.Equal(t, int64(5), ents[0].cn)
	assert.Equal(t, "0", ents[0].removed)
	assert.Contains(t, ents[0].json, `"name":"f1"`)

	st := e.w.Status()
	assert.True(t, st.Healthy)
	assert.Equal(t, int64(0), st.LagMs)
	assert.Equal(t, int64(0), st.FailedRounds)
}

func TestWriterRedisUnreachable(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(int, wtCall) error { return errors.New("connection refused") }
	gate := make(chan time.Duration)
	e.w.wait = func(d time.Duration) bool {
		e.clock.Add(d)
		select {
		case gate <- d:
			return true
		case <-e.w.stop:
			return false
		}
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	e.w.Start(nil)
	rounds := 0
	for rounds < 2 {
		if <-gate == e.opts.BackoffMaxWait {
			rounds++
		}
	}

	st := e.w.Status()
	assert.False(t, st.Healthy)
	assert.GreaterOrEqual(t, st.FailedRounds, int64(2))
	assert.Contains(t, st.LastError, "connection refused")
	assert.Equal(t, 1, st.PendingChanges, "the queued change must not be dropped while Redis is down")
	assert.Greater(t, st.LagMs, int64(0))

	e.clock.Add(time.Minute)
	later := e.w.Status()
	assert.GreaterOrEqual(t, later.LagMs-st.LagMs, int64(60000), "lag grows with the clock")

	start := time.Now()
	e.stop()
	assert.Less(t, time.Since(start), 2*time.Second, "Stop must return promptly")
	assert.Equal(t, e.log.errorCount(), int(e.w.Status().FailedRounds), "one Error per failed round")
}

func TestWriterRecoversAfterOutage(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(n int, _ wtCall) error {
		if n <= 5 {
			return errors.New("down")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	e.w.Start(nil)
	wtWaitUntil(t, "recovery", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 6, e.client.count(), "five failed tries and one applied write")
	st := e.w.Status()
	assert.True(t, st.Healthy)
	assert.Equal(t, int64(1), st.FailedRounds)
	assert.Equal(t, int64(0), st.LagMs)
	assert.Equal(t, e.clock.Now().UnixMilli(), st.LastSuccessUnixMs)
	assert.Equal(t, 1, e.log.errorCount())
}

func TestWriterPartialFailure(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 2
	e := newWtEnv(t, opts)
	failures := 0
	e.client.fail = func(_ int, c wtCall) error {
		if wtEntities(c.args)[0].name == "f2" && failures < 7 {
			failures++
			return errors.New("timeout")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f0", "f1", "f2", "f3", "f4"), nil, 7)
	e.w.Start(nil)
	wtWaitUntil(t, "the three chunks", func() bool { return e.client.count() == 10 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	firstChunk := 0
	for _, c := range calls {
		ents := wtEntities(c.args)
		if ents[0].name == "f0" {
			firstChunk++
		}
		if ents[len(ents)-1].name == "f4" {
			assert.Equal(t, int64(7), wtFlagCursor(c.args), "only the last chunk carries the real cursor")
		} else {
			assert.Equal(t, int64(-1), wtFlagCursor(c.args), "chunks before the last keep the cursor")
		}
	}
	assert.Equal(t, 1, firstChunk, "the first chunk is not re-sent after success")
	assert.Equal(t, int64(2), e.w.Status().FailedRounds)
}

func TestWriterCrashBetweenDataAndCursor(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 2
	e := newWtEnv(t, opts)
	reached := make(chan struct{})
	proceed := make(chan struct{})
	e.client.fail = func(n int, _ wtCall) error {
		if n == 2 {
			close(reached)
			<-proceed
		}
		return nil
	}
	flags := wtFlags("f0", "f1", "f2", "f3", "f4")
	e.w.PutFlags(flags, nil, 7)
	e.w.Start(nil)
	<-reached

	stopped := make(chan struct{})
	go func() {
		e.stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	close(proceed)
	<-stopped

	first := e.client.evals()
	require.Len(t, first, 2, "chunks 1 and 2 only")
	for _, c := range first {
		assert.Equal(t, int64(-1), wtFlagCursor(c.args), "no call before the stop carries the real cursor")
	}

	w2, err := NewWriter(e.client, e.w.keys, wtFingerprint, opts, e.log)
	require.NoError(t, err)
	w2.now = e.clock.Now
	w2.wait = func(time.Duration) bool { return !w2.stopped() }
	src := &wtSource{flags: flags, flagsCn: 7}
	w2.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 6 && w2.Status().PendingChanges == 0 })
	w2.Stop()

	var cursors []int64
	for _, c := range e.client.evals() {
		if slices.Equal(c.keys, e.w.keys.FlagKeys()) {
			cursors = append(cursors, wtFlagCursor(c.args))
		}
	}
	assert.Equal(t, []int64{-1, -1, -1, -1, 7}, cursors, "the real cursor only ever rides the last chunk, after its data")
}

func TestWriterEnvironmentMismatch(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	gate := make(chan struct{})
	e.client.fail = func(n int, _ wtCall) error {
		if n == 1 {
			<-gate
			return errors.New("redis error reply: PROXY_ENV_MISMATCH")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("a"), nil, 1)
	e.w.Start(nil)
	e.w.PutFlags(wtFlags("b"), nil, 2)
	e.w.PutFlags(wtFlags("c"), nil, 3)
	close(gate)
	wtWaitUntil(t, "the mismatch", func() bool { return e.w.Status().EnvironmentMismatch })

	e.w.PutFlags(wtFlags("d"), nil, 4)
	e.w.PutSegment("s", []string{"k"}, nil, 4)
	e.stop()

	assert.Equal(t, 1, e.client.count(), "no retry and nothing sent after the mismatch")
	st := e.w.Status()
	assert.True(t, st.EnvironmentMismatch)
	assert.Equal(t, 0, st.PendingChanges, "queue cleared")
	assert.Equal(t, int64(2), st.RefusedChanges)
	assert.Equal(t, int64(0), st.FailedRounds)
}

func TestWriterOverflowTriggersResync(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 1)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 2)

	st := e.w.Status()
	assert.Equal(t, int64(1), st.Resyncs)
	assert.Equal(t, 0, st.PendingChanges, "queue cleared")
	assert.Equal(t, 1, e.log.errorCount(), "the dropped count is logged")

	src := &wtSource{
		flags:   wtFlags("f1"),
		flagsCn: 3,
		segs: []SegmentState{
			{Name: "A", Active: wtKeyMap(wtKeysOf(6, "a"), 1), ChangeNumber: 1},
			{Name: "B", Active: wtKeyMap(wtKeysOf(6, "b"), 2), ChangeNumber: 2},
		},
	}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 4 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 4, e.client.count(), "flags, rule-based segments and both segments")
	assert.Equal(t, int64(1), e.w.Status().Resyncs, "the startup resync is not counted")
	assert.Equal(t, 1, src.flagsCalls)
	assert.Equal(t, 1, e.log.errorCount())
}

func wtKeyMap(keys []string, cn int64) map[string]int64 {
	m := make(map[string]int64, len(keys))
	for _, k := range keys {
		m[k] = cn
	}
	return m
}

func TestWriterRejectsBadSegmentName(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	for _, name := range []string{"", "a*", "b?", "c[1]", `d\e`} {
		e.w.PutSegment(name, []string{"k"}, nil, 1)
	}
	st := e.w.Status()
	assert.Equal(t, int64(5), st.RejectedChanges)
	assert.Equal(t, 5, e.log.errorCount())
	assert.Equal(t, 0, st.PendingChanges)
	assert.Equal(t, 0, e.client.count())
}

func TestWriterPutDoesNotBlock(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	e.client.fail = func(n int, _ wtCall) error {
		if n == 1 {
			entered <- struct{}{}
			<-release
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	e.w.Start(nil)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker never reached Eval")
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			e.w.PutFlags(wtFlags("x"), nil, 2)
			e.w.PutSegment("s", []string{"k"}, nil, 2)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Put* blocked while Eval was blocked")
	}
	releaseOnce.Do(func() { close(release) })
	e.stop()
}

func TestWriterSingleLargeChangeNotOverflow(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 100
	e := newWtEnv(t, opts)
	e.w.PutSegment("big", wtKeysOf(300, "k"), nil, 5)
	e.w.Start(nil)
	wtWaitUntil(t, "the big write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	st := e.w.Status()
	assert.Equal(t, int64(0), st.Resyncs)
	assert.Equal(t, 0, e.log.errorCount())
	name, _, adds, _ := wtSegArgs(e.client.evals()[0].args)
	assert.Equal(t, "big", name)
	assert.Len(t, adds, 300)
}

func TestWriterSegmentEmptyChangeWritesCursor(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutSegment("empty", nil, nil, 42)
	e.w.Start(nil)
	wtWaitUntil(t, "the cursor write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	call := e.client.evals()[0]
	assert.Equal(t, SegmentScript, call.script)
	assert.Equal(t, e.w.keys.SegmentKeys("empty"), call.keys)
	assert.Equal(t, e.w.keys.SegmentCursorField("empty"), call.args[4])
	name, cursor, adds, rems := wtSegArgs(call.args)
	assert.Equal(t, "empty", name)
	assert.Equal(t, int64(42), cursor)
	assert.Empty(t, adds)
	assert.Empty(t, rems)
}

func TestWriterSkippedEntityDoesNotAdvanceCursor(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		e.w.marshal = func(v interface{}) ([]byte, error) {
			if d, ok := v.(dtos.SplitDTO); ok && d.Name == "bad" {
				return nil, errors.New("boom")
			}
			return json.Marshal(v)
		}
		e.w.PutFlags(wtFlags("good", "bad"), nil, 7)
		e.w.Start(nil)
		wtWaitUntil(t, "the write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
		e.stop()

		call := e.client.evals()[0]
		ents := wtEntities(call.args)
		require.Len(t, ents, 1)
		assert.Equal(t, "good", ents[0].name)
		assert.Equal(t, int64(-1), wtFlagCursor(call.args))
		assert.Equal(t, int64(1), e.w.Status().RejectedChanges)
		assert.GreaterOrEqual(t, e.log.errorCount(), 1)
	})

	t.Run("bad args chunk", func(t *testing.T) {
		opts := wtOptions()
		opts.ChunkSize = 1
		e := newWtEnv(t, opts)
		e.client.fail = func(_ int, c wtCall) error {
			if wtEntities(c.args)[0].name == "b" {
				return errors.New("PROXY_BAD_ARGS")
			}
			return nil
		}
		e.w.PutFlags(wtFlags("a", "b", "c"), nil, 7)
		e.w.Start(nil)
		wtWaitUntil(t, "the write", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
		e.stop()

		for _, c := range e.client.evals() {
			assert.Equal(t, int64(-1), wtFlagCursor(c.args))
		}
		assert.Equal(t, int64(1), e.w.Status().RejectedChanges)
		assert.Equal(t, int64(0), e.w.Status().FailedRounds)
	})
}

func TestWriterLagCoversResync(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	src := &wtSource{onFlags: func() {
		close(entered)
		<-release
	}}
	e.w.Start(src)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the resync never started")
	}

	e.clock.Add(5 * time.Minute)
	assert.GreaterOrEqual(t, e.w.Status().LagMs, int64(5*60*1000), "lag counts the running resync")

	releaseOnce.Do(func() { close(release) })
	wtWaitUntil(t, "the resync to finish", func() bool { return e.w.Status().LagMs == 0 })
	e.stop()
}

func TestWriterBurstWithinCapNoResync(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 100
	e := newWtEnv(t, opts)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	e.client.fail = func(n int, _ wtCall) error {
		if n == 1 {
			<-release
		}
		return nil
	}
	e.w.PutSegment("s0", wtKeysOf(5, "k"), nil, 1)
	e.w.Start(nil)
	wtWaitUntil(t, "the first write to block", func() bool { return e.client.count() == 1 })
	for i := 1; i < 10; i++ {
		e.w.PutSegment(fmt.Sprintf("s%d", i), wtKeysOf(5, "k"), nil, 1)
	}
	assert.Equal(t, 10, e.w.Status().PendingChanges)

	releaseOnce.Do(func() { close(release) })
	wtWaitUntil(t, "all writes", func() bool { return e.client.count() == 10 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, int64(0), e.w.Status().Resyncs)
	assert.Equal(t, 0, e.log.errorCount())
}

func TestWriterOverflowDuringResyncSchedulesOnePass(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	calls := 0
	src := &wtSource{onFlags: func() {
		calls++
		if calls == 1 {
			close(entered)
			<-release
		}
	}}
	e.w.Start(src)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the resync never started")
	}

	for i := 0; i < 6; i++ {
		e.w.PutSegment(fmt.Sprintf("s%d", i), wtKeysOf(6, "k"), nil, 1)
	}
	assert.Equal(t, int64(3), e.w.Status().Resyncs, "every overflow is counted")

	releaseOnce.Do(func() { close(release) })
	wtWaitUntil(t, "the extra pass", func() bool { return e.client.count() == 4 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 2, calls, "exactly one pass after the running resync")
	assert.Equal(t, 4, e.client.count(), "two passes of flags and rule-based segments")
}

func TestWriterRemovalDecidedByStatusNotList(t *testing.T) {
	active := wtFlag("on", 4, "ACTIVE")
	archived := wtFlag("off", 5, "ARCHIVED")
	run := func(toAdd, toRemove []dtos.SplitDTO) []wtEnt {
		e := newWtEnv(t, wtOptions())
		e.w.PutFlags(toAdd, toRemove, 9)
		e.w.Start(nil)
		wtWaitUntil(t, "the write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
		e.stop()
		ents := wtEntities(e.client.evals()[0].args)
		slices.SortFunc(ents, func(a, b wtEnt) int { return strings.Compare(a.name, b.name) })
		return ents
	}
	correct := run([]dtos.SplitDTO{active}, []dtos.SplitDTO{archived})
	swapped := run([]dtos.SplitDTO{archived}, []dtos.SplitDTO{active})
	assert.Equal(t, correct, swapped, "swapped lists must give the same entities")
	require.Len(t, correct, 2)
	assert.Equal(t, "off", correct[0].name)
	assert.Equal(t, "1", correct[0].removed, "ARCHIVED is removed")
	assert.Equal(t, "on", correct[1].name)
	assert.Equal(t, "0", correct[1].removed, "ACTIVE is not removed")
}

func TestWriterWorkerDoesNotHoldMutexAcrossSource(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	var facadeMu sync.Mutex
	inFlags := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var enteredOnce sync.Once
	src := &wtSource{onFlags: func() {
		facadeMu.Lock()
		facadeMu.Unlock()
		enteredOnce.Do(func() { close(inFlags) })
		<-release
	}}
	e.w.Start(src)
	select {
	case <-inFlags:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker never entered Source.Flags")
	}

	putDone := make(chan struct{})
	go func() {
		facadeMu.Lock()
		e.w.PutFlags(wtFlags("f1"), nil, 2)
		facadeMu.Unlock()
		close(putDone)
	}()
	select {
	case <-putDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Put* waited while Source.Flags was blocked: the worker holds the writer mutex across Source.Flags")
	}

	releaseOnce.Do(func() { close(release) })
	wtWaitUntil(t, "the queued write", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()
}

func TestWriterRefusedLiveChunkSchedulesResync(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 1
	e := newWtEnv(t, opts)
	e.client.fail = func(_ int, c wtCall) error {
		ents := wtEntities(c.args)
		if len(ents) == 1 && ents[0].name == "b" {
			return errors.New("PROXY_BAD_ARGS")
		}
		return nil
	}
	src := &wtSource{}
	e.w.PutFlags(wtFlags("a", "b"), nil, 7)
	e.w.Start(src)
	wtWaitUntil(t, "the live write and the resync pass", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 2, src.flagsCalls, "the startup resync and one resync scheduled by the refused chunk")
	assert.Equal(t, int64(1), e.w.Status().Resyncs)
	assert.Equal(t, int64(1), e.w.Status().RejectedChanges)
}

// wtHasArg reports whether a script call carries v as a whole argument (entity, key or segment name)
func wtHasArg(args []interface{}, v string) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && s == v {
			return true
		}
	}
	return false
}

func TestWriterRefusedChunkIsolatesBadEntity(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(_ int, c wtCall) error {
		if wtHasArg(c.args, "bad") {
			return errors.New("PROXY_BAD_ARGS")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("a", "bad", "c"), nil, 7)
	e.w.Start(nil)
	wtWaitUntil(t, "the write", func() bool { return e.client.count() == 4 && e.w.Status().PendingChanges == 0 })
	e.stop()

	var written []string
	for _, c := range e.client.evals() {
		if wtHasArg(c.args, "bad") {
			continue
		}
		for _, en := range wtEntities(c.args) {
			written = append(written, en.name)
		}
	}
	assert.Equal(t, []string{"a", "c"}, written, "the good entities are written one by one")
	for _, c := range e.client.evals() {
		if wtHasArg(c.args, "bad") {
			continue
		}
		assert.Equal(t, int64(-1), wtFlagCursor(c.args), "no accepted call carries the cursor once an entity was skipped")
	}
	assert.Equal(t, int64(1), e.w.Status().RejectedChanges, "only the refused entity is counted as skipped")
	assert.Equal(t, int64(0), e.w.Status().FailedRounds)

	e.log.mu.Lock()
	msgs := append([]string(nil), e.log.errors...)
	e.log.mu.Unlock()
	require.Len(t, msgs, 2, "the refused entity and the skipped data are logged")
	assert.Contains(t, msgs[0], `"bad"`)
}

func TestWriterOverflowWithoutSourceDoesNotScheduleResync(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 1)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 2)

	e.w.mtx.Lock()
	canResync := e.w.canResync
	needResync := e.w.needResync
	e.w.mtx.Unlock()
	assert.False(t, canResync, "no source is configured")
	assert.False(t, needResync, "no resync is scheduled without a source")
	assert.Equal(t, int64(1), e.w.Status().Resyncs, "the overflow is still counted")

	e.log.mu.Lock()
	msgs := append([]string(nil), e.log.errors...)
	e.log.mu.Unlock()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "no resync is scheduled because no source is configured")
	assert.NotContains(t, msgs[0], "is scheduled from the local state")
}
