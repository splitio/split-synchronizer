package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wtFingerprint = "fp-test"

// purgeLastField is the field name test fakes use to hold the PurgeScript count that Client.Get returns
const purgeLastField = "purgeLast"

type wtCall struct {
	script string
	keys   []string
	args   []interface{}
}

// wtClient is a fake Client that records every call. fail is consulted for each Eval (n is the 1-based call number)
// and may block; it is set before Start, so the worker reads it without a lock.
type wtClient struct {
	mu         sync.Mutex
	calls      []wtCall
	hgets      []string
	purgeHget  []string
	purgeCalls []wtCall
	purgeFail  func(n int, c wtCall) error
	checks     []wtCall
	checkFail  func() error
	meta       map[string]string
	fail       func(n int, c wtCall) error
	scans      int
	deleted    []string
	delCalls   int
	delFail    func(n int, key string) error
	// probes counts the Start probe calls; probeFail, when set, decides the probe result (nil means success)
	probes    int
	probeFail func() error
}

func (c *wtClient) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	if script == startProbeScript {
		c.probes++
		fail := c.probeFail
		c.mu.Unlock()
		if fail != nil {
			return fail()
		}
		return nil
	}
	if script == SegmentCheckScript {
		c.checks = append(c.checks, wtCall{script: script, keys: keys, args: args})
		fail := c.checkFail
		c.mu.Unlock()
		if fail != nil {
			return fail()
		}
		return nil
	}
	call := wtCall{script: script, keys: keys, args: args}
	if script == PurgeScript {
		// startup purge traffic is kept apart from the resync calls that evals and count report
		c.purgeCalls = append(c.purgeCalls, call)
		// the real script sets purgeLast to the number of entries it removed; this fake removes none
		if c.meta == nil {
			c.meta = map[string]string{}
		}
		c.meta[purgeLastField] = "0"
		n := len(c.purgeCalls)
		fail := c.purgeFail
		c.mu.Unlock()
		if fail != nil {
			return fail(n, call)
		}
		return nil
	}
	c.calls = append(c.calls, call)
	n := len(c.calls)
	c.mu.Unlock()
	if c.fail != nil {
		return c.fail(n, call)
	}
	return nil
}

// fromStartupPurge reports whether the calling stack runs under Writer.startupPurge (the purge meta reads)
func fromStartupPurge() bool {
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(1, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, ".(*Writer).startupPurge") {
			return true
		}
		if !more {
			return false
		}
	}
}

func (c *wtClient) HGetAll(key string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fromStartupPurge() {
		c.purgeHget = append(c.purgeHget, key)
	} else {
		c.hgets = append(c.hgets, key)
	}
	out := map[string]string{}
	// only the meta hash has content in this fake; the data hashes read by resync are empty
	if !strings.HasSuffix(key, ".meta") {
		return out, nil
	}
	for k, v := range c.meta {
		out[k] = v
	}
	return out, nil
}

func (c *wtClient) Prefix() string { return "" }

// Get returns the purge count the fake PurgeScript recorded in the purgeLast meta field; the key is not checked
func (c *wtClient) Get(key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.meta[purgeLastField]
	if !ok {
		return "", errors.New("redis: nil")
	}
	return v, nil
}

func (c *wtClient) Scan(uint64, string, int64) ([]string, uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scans++
	return nil, 0, nil
}

// Del records every key whose DEL succeeds, in call order. delFail is consulted before each key (n is the 1-based call number).
func (c *wtClient) Del(keys ...string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var n int64
	for _, k := range keys {
		c.delCalls++
		if c.delFail != nil {
			if err := c.delFail(c.delCalls, k); err != nil {
				return n, err
			}
		}
		c.deleted = append(c.deleted, k)
		n++
	}
	return n, nil
}

func wtResetFixture(t *testing.T, c *wtClient) (*wtEnv, []string) {
	t.Helper()
	opts := wtOptions()
	e := &wtEnv{t: t, opts: opts, client: c, clock: newWtClock(), log: &wtLogger{}}
	w, err := NewWriter(c, wtKeys(t), wtFingerprint, opts, e.log)
	require.NoError(t, err)
	e.w = w
	c.meta = map[string]string{"flags.till": "1", "rbs.till": "1", "seg.s1.till": "1", "seg.s2.till": "-1"}
	u := e.w.keys.Unprefixed()
	want := []string{
		u.PurgeLast(), u.FlagData(), u.FlagIndex(), u.FlagRemoved(),
		u.RuleBasedSegmentData(), u.RuleBasedSegmentIndex(), u.RuleBasedSegmentRemoved(),
		u.SegmentActive("s1"), u.SegmentTombstones("s1"), u.SegmentRemovalClock("s1"),
		u.SegmentActive("s2"), u.SegmentTombstones("s2"), u.SegmentRemovalClock("s2"),
		u.Meta(),
	}
	return e, want
}

func TestResetDeletesKnownKeys(t *testing.T) {
	c := &wtClient{}
	e, want := wtResetFixture(t, c)

	require.NoError(t, e.w.Reset())

	assert.Equal(t, 0, c.scans, "Reset must not SCAN")
	assert.Empty(t, c.evals(), "Reset must not run any script (no KEYS inside EVAL)")
	require.NotEmpty(t, c.deleted)
	assert.Equal(t, e.w.keys.Unprefixed().Meta(), c.deleted[len(c.deleted)-1], "meta must be deleted last")
	assert.ElementsMatch(t, want, c.deleted, "flags, rule-based segments, every segment in meta (including -1) and meta")
}

func TestResetKeepsMetaWhenDelFails(t *testing.T) {
	c := &wtClient{}
	e, want := wtResetFixture(t, c)
	meta := e.w.keys.Unprefixed().Meta()
	c.delFail = func(n int, _ string) error {
		if n == 3 {
			return errors.New("del failed")
		}
		return nil
	}

	require.Error(t, e.w.Reset())
	assert.NotContains(t, c.deleted, meta, "meta must survive a failed Reset so a rerun can find the rest")
	assert.Len(t, c.deleted, 2)

	c.delFail = nil
	require.NoError(t, e.w.Reset())

	assert.Equal(t, meta, c.deleted[len(c.deleted)-1], "meta must be deleted last")
	assert.Subset(t, c.deleted, want, "every known key is deleted after the rerun")
	assert.Subset(t, want, c.deleted, "no key is orphaned")
}

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

// purgeEvals are only the PurgeScript calls of the startup purge (evals and count leave them out)
func (c *wtClient) purgeEvals() []wtCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]wtCall(nil), c.purgeCalls...)
}

// wtLogger counts Error lines and Info lines
type wtLogger struct {
	mu       sync.Mutex
	errors   []string
	warns    []string
	infos    int
	infoMsgs []string
}

func (l *wtLogger) Error(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, fmt.Sprint(msg...))
}

func (l *wtLogger) Warning(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprint(msg...))
}

func (l *wtLogger) Info(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos++
	l.infoMsgs = append(l.infoMsgs, fmt.Sprint(msg...))
}

// infoText returns every Info message logged so far, joined by newlines
func (l *wtLogger) infoText() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.infoMsgs, "\n")
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
	permAt     int // the flags read with this number (1-based) fails with permErr, which classifies as permanent
	permErr    error
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
	if s.permErr != nil && s.flagsCalls == s.permAt {
		return nil, 0, s.permErr
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
	// the fake client has an empty Prefix(), so the keys must too (NewWriter checks they match)
	k, err := NewKeys("", "apikey-test", "")
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

// wtWaitDeadline bounds every channel wait in this file so a stuck writer fails the test instead of hanging it
const wtWaitDeadline = 5 * time.Second

// wtHold blocks a hook running on the writer goroutine until ch is closed or sent to. It must not call
// t.Fatal (not the test goroutine), so a timeout is reported with t.Error and the hook returns.
func wtHold[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(wtWaitDeadline):
		t.Errorf("timed out waiting for %s", what)
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
	n := args[4].(int)
	out := []wtEnt{}
	for i := 0; i < n; i++ {
		b := 5 + 4*i
		out = append(out, wtEnt{name: args[b].(string), cn: args[b+1].(int64), removed: args[b+2].(string), json: args[b+3].(string)})
	}
	return out
}

func wtFlagCursor(args []interface{}) int64 {
	return args[3].(int64)
}

type wtSegKey struct {
	key string
	cn  int64
}

// wtSegArgs decodes a segment script call: name, cursor, adds, removes
func wtSegArgs(args []interface{}) (string, int64, []wtSegKey, []wtSegKey) {
	name := args[2].(string)
	cursor := args[4].(int64)
	nAdd := args[5].(int)
	nRem := args[6].(int)
	var adds, rems []wtSegKey
	for i := 0; i < nAdd; i++ {
		b := 7 + 2*i
		adds = append(adds, wtSegKey{key: args[b].(string), cn: args[b+1].(int64)})
	}
	for i := 0; i < nRem; i++ {
		b := 7 + 2*nAdd + 2*i
		rems = append(rems, wtSegKey{key: args[b].(string), cn: args[b+1].(int64)})
	}
	return name, cursor, adds, rems
}

func TestEntityArgsLayout(t *testing.T) {
	args := buildEntityArgs("fp", 200, "flags.till", 7, []entity{
		{name: "a", cn: 5, removed: false, json: "{}"},
		{name: "b", cn: 6, removed: true, json: "[]"},
	})
	assert.Equal(t, []interface{}{"fp", int64(200), "flags.till", int64(7), 2,
		"a", int64(5), "0", "{}",
		"b", int64(6), "1", "[]",
	}, args)
}

func TestSegmentArgsLayout(t *testing.T) {
	args := buildSegmentArgs("fp", 200, "s", "seg.s.till", 9, []segmentKey{{key: "a", cn: 1}}, []segmentKey{{key: "b", cn: 2}})
	assert.Equal(t, []interface{}{"fp", int64(200), "s", "seg.s.till", int64(9), 1, 1,
		"a", int64(1), "b", int64(2),
	}, args)
}

func TestDecodeRoundTripsWriterOutput(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	flagActive := wtFlag("f1", 5, statusActive)
	flagArchived := wtFlag("f2", 6, statusArchived)
	rbsActive := dtos.RuleBasedSegmentDTO{Name: "r1", ChangeNumber: 4, Status: statusActive}
	rbsArchived := dtos.RuleBasedSegmentDTO{Name: "r2", ChangeNumber: 8, Status: statusArchived}
	e.w.PutFlags([]dtos.SplitDTO{flagActive}, []dtos.SplitDTO{flagArchived}, 6)
	e.w.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{rbsActive}, []dtos.RuleBasedSegmentDTO{rbsArchived}, 8)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "both entity writes", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.stop()

	gotFlags := map[string]dtos.SplitDTO{}
	gotRbs := map[string]dtos.RuleBasedSegmentDTO{}
	for _, c := range e.client.evals() {
		for _, en := range wtEntities(c.args) {
			removed := en.removed == "1"
			if c.keys[2] == e.w.keys.FlagIndex() {
				d, err := DecodeFlag([]byte(en.json), en.cn, removed)
				require.Nil(t, err)
				gotFlags[en.name] = d
			} else {
				d, err := DecodeRuleBasedSegment([]byte(en.json), en.cn, removed)
				require.Nil(t, err)
				gotRbs[en.name] = d
			}
		}
	}
	assert.Equal(t, map[string]dtos.SplitDTO{"f1": flagActive, "f2": flagArchived}, gotFlags)
	assert.Equal(t, map[string]dtos.RuleBasedSegmentDTO{"r1": rbsActive, "r2": rbsArchived}, gotRbs)
}

func TestDecodeScoreOverridesChangeNumberInJSON(t *testing.T) {
	d, err := DecodeFlag([]byte(`{"name":"f","changeNumber":1,"status":"ACTIVE"}`), 42, false)
	require.Nil(t, err)
	assert.Equal(t, int64(42), d.ChangeNumber)
	assert.Equal(t, statusActive, d.Status)
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	_, err := DecodeFlag([]byte(`{`), 1, false)
	assert.NotNil(t, err)
	_, err = DecodeRuleBasedSegment([]byte(`{`), 1, false)
	assert.NotNil(t, err)
}

func TestParseCursor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		want  int64
		isErr bool
	}{
		{name: "positive", in: "42", want: 42},
		{name: "negative sentinel", in: "-2", want: -2},
		{name: "empty", in: "", isErr: true},
		{name: "not a number", in: "abc", isErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCursor(tc.in)
			if tc.isErr {
				assert.True(t, errors.Is(err, strconv.ErrSyntax), "%v", err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
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

func TestWriterDefaultOptions(t *testing.T) {
	keys := wtKeys(t)
	o := DefaultOptions()
	assert.Equal(t, 3, o.MaxAttempts)
	assert.Equal(t, 60*time.Second, o.BackoffMaxWait)
	assert.Equal(t, 3000000, o.MaxPendingChanges)
	assert.Equal(t, 1000, o.ChunkSize)
	assert.Equal(t, 500, o.PurgeBatch)
	assert.Equal(t, int64(10000), o.StopTimeoutMs)

	_, err := NewWriter(&wtClient{}, keys, wtFingerprint, Options{}, &wtLogger{})
	assert.ErrorIs(t, err, ErrInvalidOptions, "zero Options must be rejected")

	bounds := map[string]func(o *Options){
		"chunk size max":   func(o *Options) { o.ChunkSize = 10000 },
		"purge batch max":  func(o *Options) { o.PurgeBatch = 5000 },
		"stop timeout max": func(o *Options) { o.StopTimeoutMs = 60000 },
	}
	for name, set := range bounds {
		t.Run(name, func(t *testing.T) {
			o := DefaultOptions()
			set(&o)
			_, err := NewWriter(&wtClient{}, keys, wtFingerprint, o, &wtLogger{})
			assert.NoError(t, err)
		})
	}

	rejected := map[string]func(o *Options){
		"chunk size 0":       func(o *Options) { o.ChunkSize = 0 },
		"chunk size -1":      func(o *Options) { o.ChunkSize = -1 },
		"chunk size max+1":   func(o *Options) { o.ChunkSize = 10001 },
		"purge batch 0":      func(o *Options) { o.PurgeBatch = 0 },
		"purge batch -1":     func(o *Options) { o.PurgeBatch = -1 },
		"purge batch max+1":  func(o *Options) { o.PurgeBatch = 5001 },
		"stop timeout 0":     func(o *Options) { o.StopTimeoutMs = 0 },
		"stop timeout -1":    func(o *Options) { o.StopTimeoutMs = -1 },
		"stop timeout max+1": func(o *Options) { o.StopTimeoutMs = 60001 },
	}
	for name, mutate := range rejected {
		t.Run(name, func(t *testing.T) {
			o := DefaultOptions()
			mutate(&o)
			_, err := NewWriter(&wtClient{}, keys, wtFingerprint, o, &wtLogger{})
			assert.ErrorIs(t, err, ErrInvalidOptions)
		})
	}
}

func TestWriterSuccess(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("f1", 5, "ACTIVE")}, nil, 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, 1)
	assert.Equal(t, FlagScript, calls[0].script)
	assert.Equal(t, e.w.keys.FlagKeys(), calls[0].keys)
	args := calls[0].args
	assert.Equal(t, wtFingerprint, args[0])
	assert.Len(t, args, 5+4, "ARGV carries no clock: the script reads Redis TIME")
	assert.Equal(t, e.opts.Retention.Milliseconds(), args[1])
	assert.Equal(t, e.w.keys.FlagCursorField(), args[2])
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
	e.client.fail = func(int, wtCall) error {
		return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	// the change gets MaxAttempts tries, then it is dropped; Redis stays behind until a resync repairs it
	wtWaitUntil(t, "the drop", func() bool { return e.w.Status().FailedRounds == 1 && e.w.Status().PendingChanges == 0 })

	assert.Equal(t, e.opts.MaxAttempts, e.client.count(), "the change is tried MaxAttempts times, then dropped")
	st := e.w.Status()
	assert.False(t, st.Healthy)
	assert.Equal(t, int64(1), st.FailedRounds)
	assert.Contains(t, st.LastError, "connection refused")
	assert.Equal(t, 0, st.PendingChanges, "the change is dropped, Redis behind the source")
	assert.Equal(t, 1, e.log.errorCount(), "one Error line for the dropped change")

	start := time.Now()
	e.stop()
	assert.Less(t, time.Since(start), 2*time.Second, "Stop must return promptly")
}

func TestWriterRecoversAfterOutage(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(n int, _ wtCall) error {
		if n <= 3 {
			return &net.OpError{Op: "dial", Err: errors.New("down")}
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	// evals 1-3 fail: the first change is dropped after MaxAttempts
	wtWaitUntil(t, "the drop", func() bool { return e.w.Status().FailedRounds == 1 && e.w.Status().PendingChanges == 0 })
	assert.Equal(t, 3, e.client.count(), "the first change gets MaxAttempts tries, then it is dropped")
	assert.Equal(t, 1, e.log.errorCount(), "one Error line for the drop")

	// the outage is over: eval 4 succeeds for the second change
	e.w.PutFlags(wtFlags("f2"), nil, 2)
	wtWaitUntil(t, "recovery", func() bool { return e.w.Status().Healthy && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 4, e.client.count(), "three failed tries for the dropped change and one applied write for the next change")
	st := e.w.Status()
	assert.True(t, st.Healthy)
	assert.Equal(t, "", st.LastError, "a successful write clears the last error")
	assert.Equal(t, int64(0), st.FailedRounds, "a successful write clears the failed rounds")
	assert.Equal(t, int64(0), st.LagMs)
	assert.Equal(t, e.clock.Now().UnixMilli(), st.LastSuccessUnixMs)
	assert.Equal(t, 1, e.log.errorCount(), "still exactly one Error line: the success logs nothing")
}

func TestWriterPartialFailure(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 2
	e := newWtEnv(t, opts)
	// the chunk holding f2 fails for the first MaxAttempts tries of the first change only
	failures := 0
	e.client.fail = func(_ int, c wtCall) error {
		if wtEntities(c.args)[0].name == "f2" && failures < opts.MaxAttempts {
			failures++
			return &net.OpError{Op: "read", Err: errors.New("i/o timeout")}
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f0", "f1", "f2", "f3", "f4"), nil, 7)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	// chunks [f0 f1] ok (eval 1), [f2 f3] fails 3 times (evals 2-4): the chunk is dropped and [f4] is never sent
	wtWaitUntil(t, "the drop", func() bool { return e.w.Status().FailedRounds == 1 && e.w.Status().PendingChanges == 0 })
	assert.Equal(t, 1+opts.MaxAttempts, e.client.count(), "one good chunk, then MaxAttempts tries of the f2 chunk; the chunk after it is not sent")
	for _, c := range e.client.evals() {
		assert.NotEqual(t, "f4", wtEntities(c.args)[len(wtEntities(c.args))-1].name, "the last chunk, the only one with the real cursor, is not sent after the drop")
		assert.Equal(t, int64(-1), wtFlagCursor(c.args), "no call before the drop carries the real cursor, so the cursor did not advance past the dropped chunk")
	}
	assert.Equal(t, 1, e.log.errorCount(), "one Error line for the dropped chunk")
	assert.False(t, e.w.Status().Healthy)

	// the outage is over: a following change writes all three chunks; the f0 chunk is sent again because the change is new
	e.w.PutFlags(wtFlags("f0", "f1", "f2", "f3", "f4"), nil, 7)
	wtWaitUntil(t, "the following change", func() bool { return e.w.Status().Healthy && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 1+opts.MaxAttempts+3, e.client.count(), "the following change writes its three chunks once each, with no further failures")
	calls := e.client.evals()
	firstChunk := 0
	for _, c := range calls {
		if wtEntities(c.args)[0].name == "f0" {
			firstChunk++
		}
		// the drop freezes the group until a resync; Start(nil) makes the resync a no-op, so even the last chunk
		// of the following change sends noCursor and the stored cursor stays behind the data
		assert.Equal(t, int64(-1), wtFlagCursor(c.args), "no call after the drop sends the real cursor while the group is frozen")
	}
	assert.Equal(t, 2, firstChunk, "the f0 chunk is sent once per change: once by the dropped change and once by the following change")
	assert.Equal(t, int64(0), e.w.Status().FailedRounds, "the write after the outage clears the failed rounds")
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
			wtHold(t, proceed, "proceed signal")
			// the drain deadline passes while chunk 2 is in flight, so the stop cuts the change short
			e.clock.Add(time.Duration(opts.StopTimeoutMs) * time.Millisecond)
		}
		return nil
	}
	flags := wtFlags("f0", "f1", "f2", "f3", "f4")
	e.w.PutFlags(flags, nil, 7)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWithinDeadline(t, "the first chunk to reach Redis", reached, 5*time.Second)

	stopped := make(chan struct{})
	go func() {
		e.stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	close(proceed)
	wtWithinDeadline(t, "Stop to return", stopped, 5*time.Second)

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
	if err := w2.Start(src); err != nil {
		t.Fatal(err)
	}
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
			wtHold(t, gate, "gate signal")
			return errors.New("redis error reply: PROXY_ENV_MISMATCH")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("a"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
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
	assert.False(t, st.Healthy)
	assert.Equal(t, ErrEnvironmentMismatch.Error(), st.LastError)
	assert.Equal(t, 0, st.PendingChanges, "queue cleared")
	assert.Equal(t, int64(2), st.RefusedChanges)
	assert.Equal(t, int64(0), st.FailedRounds)
}

func TestSuccessClearsLastError(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(n int, _ wtCall) error {
		if n <= 3 {
			return errors.New("BUSY redis is busy")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("a"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	// evals 1-3 are BUSY: the first change is dropped after MaxAttempts, so the failed round is a drop
	wtWaitUntil(t, "the drop", func() bool { return e.w.Status().FailedRounds == 1 && e.w.Status().PendingChanges == 0 })
	st := e.w.Status()
	assert.False(t, st.Healthy, "the dropped change leaves the writer unhealthy")
	assert.Contains(t, st.LastError, "BUSY redis is busy", "the drop records the last error")
	assert.Equal(t, 3, e.client.count(), "the first change gets MaxAttempts tries, then it is dropped")

	// eval 4 succeeds for the next change: the success clears the error of the dropped change
	e.w.PutFlags(wtFlags("b"), nil, 2)
	wtWaitUntil(t, "the write after the drop", func() bool { return e.w.Status().Healthy && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 4, e.client.count(), "three BUSY tries, then one successful write of the next change")
	st = e.w.Status()
	assert.True(t, st.Healthy)
	assert.Equal(t, "", st.LastError, "the successful write clears LastError")
	assert.Equal(t, int64(0), st.FailedRounds, "the successful write clears FailedRounds")
}

func TestResetClearsHealthAndLastError(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.recordDrop("flag write", 3, errors.New("BUSY redis is busy"))
	require.False(t, e.w.Status().Healthy)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = e.w.Status()
			}
		}
	}()
	err := e.w.Reset()
	close(done)
	wg.Wait()
	require.NoError(t, err)

	st := e.w.Status()
	assert.True(t, st.Healthy)
	assert.Equal(t, "", st.LastError)
	assert.Equal(t, int64(0), st.FailedRounds)
}

func TestNoResyncAfterEnvironmentMismatch(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 1
	e := newWtEnv(t, opts)
	e.client.fail = func(_ int, c wtCall) error {
		for _, ent := range wtEntities(c.args) {
			switch ent.name {
			case "b":
				return errors.New("PROXY_BAD_ARGS")
			case "c":
				return errors.New("redis error reply: PROXY_ENV_MISMATCH")
			}
		}
		return nil
	}
	src := &wtSource{}
	e.w.PutFlags(wtFlags("a", "b", "c"), nil, 7)
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the mismatch", func() bool { return e.w.Status().EnvironmentMismatch })
	e.stop()

	st := e.w.Status()
	assert.Equal(t, int64(0), st.Resyncs, "a refused chunk must not schedule a resync once the environment mismatches")
	assert.Equal(t, 1, src.flagsCalls, "only the startup resync reads the Source")
}

func TestWriterOverflowTriggersResync(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 1)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 2)

	st := e.w.Status()
	assert.Equal(t, int64(0), st.Resyncs, "no source yet: an overflow without a source is not counted")
	assert.Equal(t, 1, st.PendingChanges, "queue cleared, then the triggering change B is queued")
	assert.Equal(t, 1, e.log.errorCount(), "the dropped count is logged")

	src := &wtSource{
		flags:   wtFlags("f1"),
		flagsCn: 3,
		segs: []SegmentState{
			{Name: "A", Active: wtKeyMap(wtKeysOf(6, "a"), 1), ChangeNumber: 1},
			{Name: "B", Active: wtKeyMap(wtKeysOf(6, "b"), 2), ChangeNumber: 2},
		},
	}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 5 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 5, e.client.count(), "flags, rule-based segments, both segments and the queued triggering change B")
	assert.Equal(t, int64(0), e.w.Status().Resyncs, "the startup resync is not counted")
	assert.Equal(t, 1, src.flagsCalls)
	assert.Equal(t, 1, e.log.errorCount())
}

func TestOverflowKeepsTriggeringChange(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 1)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 2)

	assert.Equal(t, 1, e.w.Status().PendingChanges, "the dropped queue is replaced by the triggering change")
	assert.Equal(t, 1, e.log.errorCount())

	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the triggering change is written", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	if assert.Len(t, calls, 1, "only the triggering change is written, A was dropped") {
		assert.Equal(t, SegmentScript, calls[0].script)
		assert.Equal(t, e.w.keys.SegmentCursorField("B"), calls[0].args[3])
	}
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

func TestSegmentRefusedKeyFreezesGroup(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 1
	e := newWtEnv(t, opts)
	e.client.fail = func(_ int, c wtCall) error {
		_, _, adds, _ := wtSegArgs(c.args)
		for _, k := range adds {
			if k.key == "bad" {
				return errors.New("PROXY_BAD_ARGS")
			}
		}
		return nil
	}
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutSegment("s", []string{"ok1"}, nil, 5)
	wtWaitUntil(t, "the first clean write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutSegment("s", []string{"bad"}, nil, 6)
	wtWaitUntil(t, "the refused write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.w.PutSegment("s", []string{"ok2"}, nil, 7)
	wtWaitUntil(t, "the second clean write", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	_, first, _, _ := wtSegArgs(calls[0].args)
	_, last, _, _ := wtSegArgs(calls[2].args)
	assert.Equal(t, int64(5), first)
	assert.Equal(t, int64(5), last, "a clean write after a refused key must not advance the cursor past the last written one")
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
			wtHold(t, release, "release signal")
		}
		return nil
	}
	e.w.PutFlags(wtFlags("f1"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the cursor write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	call := e.client.evals()[0]
	assert.Equal(t, SegmentScript, call.script)
	assert.Equal(t, e.w.keys.SegmentKeys("empty"), call.keys)
	assert.Equal(t, e.w.keys.SegmentCursorField("empty"), call.args[3])
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
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
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
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
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
		wtHold(t, release, "release signal")
	}}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
			wtHold(t, release, "release signal")
		}
		return nil
	}
	e.w.PutSegment("s0", wtKeysOf(5, "k"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
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
			wtHold(t, release, "release signal")
		}
	}}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the resync never started")
	}

	for i := 0; i < 6; i++ {
		e.w.PutSegment(fmt.Sprintf("s%d", i), wtKeysOf(6, "k"), nil, 1)
	}
	assert.Equal(t, int64(5), e.w.Status().Resyncs, "every overflow is counted (each put after the first overflows: the triggering change is kept)")

	releaseOnce.Do(func() { close(release) })
	wtWaitUntil(t, "the extra pass", func() bool { return e.client.count() == 5 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 2, calls, "exactly one pass after the running resync")
	assert.Equal(t, 5, e.client.count(), "two passes of flags and rule-based segments, plus the queued triggering change")
}

func TestWriterWorkerDoesNotHoldMutexAcrossSource(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	inFlags := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var enteredOnce sync.Once
	src := &wtSource{onFlags: func() {
		enteredOnce.Do(func() { close(inFlags) })
		wtHold(t, release, "release signal")
	}}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-inFlags:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker never entered Source.Flags")
	}

	putDone := make(chan struct{})
	go func() {
		e.w.PutFlags(wtFlags("f1"), nil, 2)
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the live write and the resync pass", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 2, src.flagsCalls, "the startup resync and one resync scheduled by the refused chunk")
	assert.Equal(t, int64(1), e.w.Status().Resyncs)
	assert.Equal(t, int64(1), e.w.Status().RejectedChanges)
}

// every refused live chunk schedules exactly one resync
func TestRefusedChunkSchedulesOneResync(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 1
	e := newWtEnv(t, opts)
	e.client.fail = func(_ int, c wtCall) error {
		ents := wtEntities(c.args)
		if len(ents) == 1 && (ents[0].name == "b" || ents[0].name == "c") {
			return errors.New("PROXY_BAD_ARGS")
		}
		return nil
	}
	pending := func() bool {
		e.w.mtx.Lock()
		defer e.w.mtx.Unlock()
		return e.w.needResync
	}
	e.w.PutFlags(wtFlags("a", "b"), nil, 7)
	if err := e.w.Start(&wtSource{}); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the first refused chunk and its resync", func() bool {
		return e.w.Status().Resyncs == 1 && e.w.Status().PendingChanges == 0 && !pending()
	})
	e.w.PutFlags(wtFlags("c"), nil, 8)
	wtWaitUntil(t, "the second refused chunk and its resync", func() bool { return e.w.Status().Resyncs == 2 })
	e.stop()

	assert.Equal(t, int64(2), e.w.Status().RejectedChanges, "each live chunk is refused once")
	assert.Equal(t, int64(2), e.w.Status().Resyncs, "one resync per refused live chunk")
}

func TestInvalidSegmentNameSkipsEntity(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutSegment("", []string{"k0"}, nil, 3)
	e.w.PutSegment("a*b", []string{"k1"}, nil, 3)
	e.w.PutSegment("seg[1]", []string{"k2"}, nil, 3)
	e.w.PutSegment("valid", []string{"k3"}, nil, 3)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the valid segment write", func() bool { return e.client.count() >= 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags(wtFlags("f"), nil, 4)
	wtWaitUntil(t, "the flag write after the invalid names", func() bool { return e.client.count() >= 2 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, int64(3), e.w.Status().RejectedChanges, "each invalid name is rejected once")
	assert.Equal(t, 3, e.log.errorCount(), "one Error per invalid name")
	var segNames []string
	var segCursors, flagCursors []int64
	for _, c := range e.client.evals() {
		switch c.script {
		case SegmentScript:
			name, cur, _, _ := wtSegArgs(c.args)
			segNames = append(segNames, name)
			segCursors = append(segCursors, cur)
		case FlagScript:
			flagCursors = append(flagCursors, wtFlagCursor(c.args))
		}
	}
	assert.Equal(t, []string{"valid"}, segNames, "only the valid segment is written")
	assert.Equal(t, []int64{3}, segCursors, "the valid segment is sent with its real cursor")
	assert.Equal(t, []int64{4}, flagCursors, "the flag group is sent with its real cursor")

	g := e.w.groups[e.w.keys.SegmentCursorField("valid")]
	if g != nil {
		assert.Empty(t, g.names, "no skip recorded for the valid segment")
		assert.False(t, g.whole, "no whole-group freeze")
	}
	for _, name := range []string{"", "a*b", "seg[1]"} {
		assert.Nil(t, e.w.groups[e.w.keys.SegmentCursorField(name)], "no group state for invalid name %q", name)
	}
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
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
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
	assert.Equal(t, int64(0), e.w.Status().Resyncs, "no resync is counted without a source")

	e.log.mu.Lock()
	msgs := append([]string(nil), e.log.errors...)
	e.log.mu.Unlock()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "no resync is scheduled because no source is configured")
	assert.NotContains(t, msgs[0], "is scheduled from the local state")
}

func TestOverflowWithoutSourceStaysFrozen(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	reached := make(chan struct{})
	gate := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	e.client.fail = func(n int, _ wtCall) error {
		if n == 1 {
			close(reached)
			wtHold(t, gate, "gate signal")
		}
		return nil
	}
	e.w.PutSegment("C", wtKeysOf(1, "c"), nil, 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the first write did not start")
	}
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 6)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 7)
	release()
	wtWaitUntil(t, "the writes", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, int64(0), e.w.Status().Resyncs, "no source is configured, so no resync is counted")
	e.log.mu.Lock()
	msgs := append([]string(nil), e.log.errors...)
	e.log.mu.Unlock()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "no resync is scheduled because no source is configured")

	calls := e.client.evals()
	require.Len(t, calls, 2)
	assert.Equal(t, int64(noCursor), calls[1].args[4], "the overflowed group stays frozen and sends no new cursor")
}

func TestOverflowFreezesCursorsUntilResync(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 10
	e := newWtEnv(t, opts)
	reached := make(chan struct{})
	gate := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	e.client.fail = func(n int, _ wtCall) error {
		if n == 1 {
			close(reached)
			wtHold(t, gate, "gate signal")
		}
		return nil
	}
	src := &wtSource{flags: wtFlags("a", "b"), flagsCn: 8, permAt: 2, permErr: errors.New("WRONGTYPE source flags unreadable")}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the startup resync did not start")
	}
	e.w.PutSegment("A", wtKeysOf(6, "a"), nil, 6)
	e.w.PutSegment("B", wtKeysOf(6, "b"), nil, 7)
	release()

	flagCursors := func() []int64 {
		var out []int64
		for _, c := range e.client.evals() {
			if c.script == FlagScript && c.args[2] == "flags.till" {
				out = append(out, c.args[3].(int64))
			}
		}
		return out
	}
	segCursors := func() []int64 {
		var out []int64
		for _, c := range e.client.evals() {
			if c.script == SegmentScript {
				out = append(out, c.args[4].(int64))
			}
		}
		return out
	}
	wtWaitUntil(t, "the frozen write after the failed resync pass", func() bool { return len(segCursors()) == 1 && e.w.Status().PendingChanges == 0 })
	e.w.RequestResync()
	e.w.PutSegment("D", wtKeysOf(6, "d"), nil, 12)
	wtWaitUntil(t, "the write after the clean resync", func() bool { return len(segCursors()) == 2 })
	e.stop()

	assert.Equal(t, []int64{int64(noCursor), 12}, segCursors(), "the overflowed group sends no cursor while the failed pass leaves the overflow set, and its real cursor after a clean resync")
	assert.Equal(t, []int64{8, 8}, flagCursors(), "the startup pass and the clean resync each write the flags cursor")
}

// the test protects the retry cap. When the MaxAttempts tries of a change are exhausted the change is dropped,
// not kept and retried; a later change is still written.
func TestWriterMaxAttemptsExhaustedDropsChange(t *testing.T) {
	opts := wtOptions()
	e := newWtEnv(t, opts)
	e.client.fail = func(n int, _ wtCall) error {
		if n <= opts.MaxAttempts {
			return &net.OpError{Op: "dial", Err: errors.New("redis down")}
		}
		return nil
	}
	e.w.PutFlags(wtFlags("a"), nil, 7)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the drop", func() bool { return e.w.Status().FailedRounds == 1 && e.w.Status().PendingChanges == 0 })

	s := e.w.Status()
	assert.False(t, s.Healthy)
	assert.Equal(t, int64(1), s.FailedRounds)
	assert.Equal(t, "dial: redis down", s.LastError)
	assert.Equal(t, 0, s.PendingChanges, "the change is dropped after MaxAttempts, it is not kept for another try")
	assert.Equal(t, 1, e.log.errorCount(), "one error line for the dropped change")
	require.Equal(t, opts.MaxAttempts, e.client.count(), "exactly MaxAttempts tries, no retry after the cap")

	e.w.PutFlags(wtFlags("b"), nil, 8)
	wtWaitUntil(t, "the later change", func() bool { return e.w.Status().Healthy && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, opts.MaxAttempts+1)
	for _, c := range calls[:opts.MaxAttempts] {
		assert.Equal(t, "a", wtEntities(c.args)[0].name, "the tries are for the dropped change")
	}
	assert.Equal(t, "b", wtEntities(calls[opts.MaxAttempts].args)[0].name, "the dropped change is not sent again; the later change is")
	assert.True(t, e.w.Status().Healthy)
	assert.Equal(t, int64(0), e.w.Status().FailedRounds, "a successful write clears the failed rounds")
}

// concurrent and repeated calls never block, even while a resync runs, and coalesce into one more pass
func TestRequestResyncConcurrentCallsCoalesce(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	entered := make(chan struct{})
	release := make(chan struct{})
	var once, releaseOnce sync.Once
	releaseGate := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseGate)
	src := newSegDoneSource(&wtSource{flags: wtFlags("a"), flagsCn: 1})
	src.onFlags = func() {
		once.Do(func() {
			close(entered)
			wtHold(t, release, "release signal")
		})
	}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the startup resync did not start")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				e.w.RequestResync()
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RequestResync blocked while the worker was busy")
	}

	releaseGate()
	wtWaitUntil(t, "the requested resync", func() bool { return src.passes.Load() >= 2 })
	e.stop()
	assert.Equal(t, int32(2), src.passes.Load(), "the startup pass and one coalesced pass")
	assert.Equal(t, int64(0), e.w.Status().Resyncs, "a requested resync is not an overflow event")
}

// before Start, without a source, after Stop and in an environment mismatch the call does nothing
func TestRequestResyncIsNoOpWithoutRunningWriter(t *testing.T) {
	pending := func(w *Writer) bool {
		w.mtx.Lock()
		defer w.mtx.Unlock()
		return w.needResync
	}
	newSrc := func() *segDoneSource { return newSegDoneSource(&wtSource{flags: wtFlags("a"), flagsCn: 1}) }

	t.Run("before Start", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		e.w.RequestResync()
		assert.False(t, pending(e.w))
		assert.Equal(t, 0, e.client.count())
	})
	t.Run("without a source", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		e.w.RequestResync()
		e.stop()
		assert.False(t, pending(e.w))
		assert.Equal(t, 0, e.client.count())
	})
	t.Run("after Stop", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		src := newSrc()
		if err := e.w.Start(src); err != nil {
			t.Fatal(err)
		}
		src.wait(t)
		e.stop()
		e.w.RequestResync()
		e.w.RequestResync()
		assert.False(t, pending(e.w))
		assert.Equal(t, int32(1), src.passes.Load())
	})
	t.Run("environment mismatch", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		e.client.meta = map[string]string{"fingerprint": "another-configuration"}
		if err := e.w.Start(newSrc()); err != nil {
			t.Fatal(err)
		}
		wtWaitUntil(t, "the mismatch", func() bool { return e.w.Status().EnvironmentMismatch })
		e.w.RequestResync()
		e.stop()
		assert.False(t, pending(e.w))
		assert.Equal(t, 0, e.client.count())
	})
}

func TestPutFlagsRemovalListOverridesActiveStatus(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags(nil, []dtos.SplitDTO{wtFlag("f1", 5, statusActive)}, 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the flag write", func() bool { return e.client.count() == 1 })
	e.stop()
	ents := wtEntities(e.client.evals()[0].args)
	require.Len(t, ents, 1)
	assert.Equal(t, "f1", ents[0].name)
	assert.Equal(t, "1", ents[0].removed, "an entity from the removal list is written removed even when ACTIVE")
}

// a rule-based segment from the removal list is written removed even when ACTIVE, and one from the add list keeps the status rule.
func TestPutRuleBasedSegmentsRemovalListOverridesActiveStatus(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{{Name: "added", ChangeNumber: 5, Status: statusActive}},
		[]dtos.RuleBasedSegmentDTO{{Name: "removed", ChangeNumber: 5, Status: statusActive}}, 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the rule-based segment write", func() bool { return e.client.count() == 1 })
	e.stop()
	ents := wtEntities(e.client.evals()[0].args)
	require.Len(t, ents, 2)
	byName := map[string]string{}
	for _, en := range ents {
		byName[en.name] = en.removed
	}
	assert.Equal(t, "1", byName["removed"], "an entity from the removal list is written removed even when ACTIVE")
	assert.Equal(t, "0", byName["added"], "an ACTIVE entity from the add list is written active")
}

// F27: 50 goroutines call the Put* methods while the worker drains; run under -race, every call returns and Stop returns.
func TestConcurrentPutsReturnUnderRace(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				name := fmt.Sprintf("f%d", g)
				e.w.PutFlags(wtFlags(name), nil, int64(i+1))
				e.w.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{{Name: name, ChangeNumber: int64(i + 1), Status: statusActive}}, nil, int64(i+1))
			}
		}(g)
	}
	putsDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(putsDone)
	}()
	wtWithinDeadline(t, "the concurrent Put calls to return", putsDone, 10*time.Second)
	e.stop()
	assert.Equal(t, int64(0), e.w.Status().RejectedChanges, "no change is rejected")
}

// error classification. The classify tests call it directly; the others run through the writer.

func TestClassifyRedisErrors(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want errClass
	}{
		{"plain BUSY", "BUSY Redis is busy running a script. You can only call SCRIPT KILL or SHUTDOWN NOSAVE.", errTransient},
		{"plain OOM", "OOM command not allowed when used memory > 'maxmemory'.", errTransient},
		{"plain READONLY", "READONLY You can't write against a read only replica.", errTransient},
		{"lua wrapped OOM wins over lua error", "ERR user_script:1: OOM command not allowed when used memory > maxmemory script: 697390339656a7da341b346b26c5ce0552223122, on @user_script:1.", errTransient},
		{"lua wrapped READONLY wins over lua error", "ERR user_script:1: READONLY You can't write against a read only replica. script: 0753be4445ca355585f921a9dcf10d124588b3b9, on @user_script:1.", errTransient},
		{"lua wrapped BUSY", "ERR Error running script (call to f_1a2b): @user_script:1: BUSY Redis is busy running a script.", errTransient},
		{"lua wrapped WRONGTYPE is permanent", "ERR user_script:1: WRONGTYPE Operation against a key holding the wrong kind of value script: 691ad66b169ddb9e0d5fb480aeb7c7887a2c4b59, on @user_script:1.", errPermanent},
		{"NOAUTH is transient", "NOAUTH Authentication required.", errTransient},
		{"unlisted text is unknown", "something Redis has never said", errUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, classify(errors.New(c.err)), c.err)
		})
	}
}

func TestWrongTypeMetaFailsRound(t *testing.T) {
	opts := wtOptions()
	e := newWtEnv(t, opts)
	meta := e.w.keys.Meta()
	e.client.fail = func(n int, c wtCall) error {
		if n <= opts.MaxAttempts && slices.Contains(c.keys, meta) {
			return errors.New("ERR user_script:1: PROXY_META_WRONGTYPE script: 0, on @user_script:1.")
		}
		return nil
	}
	err := e.w.eval("meta test", "meta-script", []string{meta}, nil)
	require.ErrorIs(t, err, errDropped, "the transient meta error is dropped after the attempt cap, not retried forever")
	assert.Equal(t, opts.MaxAttempts, e.client.count())
	assert.Len(t, e.log.errors, 1, "the drop is logged once at Error level")
	assert.Equal(t, int64(0), e.w.Status().RejectedChanges, "a meta hash WRONGTYPE must not skip anything")
}

func TestSegmentIntactWrongTypeRewrites(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.checkFail = func() error {
		return errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	}
	intact, err := e.w.segmentIntact(SegmentState{Name: "seg", Active: map[string]int64{"a": 1}})
	require.NoError(t, err)
	assert.False(t, intact, "a WRONGTYPE segment is rewritten, not treated as intact")
}

func TestSegmentIntactTransientErrorPropagates(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.checkFail = func() error { return errors.New("LOADING Redis is loading the dataset in memory") }
	waits := 0
	e.w.wait = func(time.Duration) bool {
		waits++
		return waits < 5
	}
	intact, err := e.w.segmentIntact(SegmentState{Name: "seg", Active: map[string]int64{"a": 1}})
	require.Error(t, err, "a transient error must propagate, not be read as a missing segment")
	assert.False(t, errors.Is(err, errChunkRejected), "errChunkRejected would trigger a rewrite")
	assert.False(t, intact)
	assert.GreaterOrEqual(t, len(e.client.checks), 2, "transient errors are retried before giving up")
}

func TestUnknownErrorPersistentDropsAfterMaxAttempts(t *testing.T) {
	opts := wtOptions()
	e := newWtEnv(t, opts)
	data := e.w.keys.FlagData()
	e.client.fail = func(int, wtCall) error { return errors.New("NOPERM User has no permissions to run the script") }
	err := e.w.eval("flag write", "flag-script", []string{data}, nil)
	assert.ErrorIs(t, err, errDropped, "an unknown error that persists is dropped, not rejected")
	assert.NotErrorIs(t, err, errChunkRejected, "no per-entity isolation for an unknown error")
	assert.Equal(t, opts.MaxAttempts, e.client.count(), "never retried beyond MaxAttempts, no per-entity retries")
	st := e.w.Status()
	assert.False(t, st.Healthy, "the drop leaves the writer unhealthy")
	e.client.fail = nil
	require.NoError(t, e.w.eval("flag write", "flag-script", []string{data}, nil), "the worker is not stalled by the drop")
}

// the dropped error keeps errDropped and the last Redis error, so callers can see the original class
func TestRetryDroppedErrorKeepsLastClassAndCause(t *testing.T) {
	opts := wtOptions()
	e := newWtEnv(t, opts)
	last := errors.New("redis: connection pool timeout")
	calls := 0
	err := e.w.retry("flag write", func() error {
		calls++
		return last
	})
	assert.ErrorIs(t, err, errDropped, "the transient error runs out of tries")
	assert.ErrorIs(t, err, last, "the original Redis error is still visible")
	assert.ErrorContains(t, err, "last error class transient", "the class of the last error is in the error text")
	assert.Equal(t, opts.MaxAttempts, calls, "retried MaxAttempts times")
}

func TestRetryCapDropsChange(t *testing.T) {
	opts := wtOptions()
	e := newWtEnv(t, opts)
	e.client.fail = func(_ int, c wtCall) error {
		if wtHasArg(c.args, "drop") {
			return errors.New("redis: connection pool timeout")
		}
		return nil
	}
	err := e.w.eval("flag write", "flag-script", []string{e.w.keys.FlagData()}, []interface{}{"drop"})
	assert.ErrorIs(t, err, errDropped, "a transient error is dropped after its bounded attempts")
	assert.Equal(t, opts.MaxAttempts, e.client.count(), "never retried beyond MaxAttempts")
	assert.Equal(t, 1, e.log.errorCount(), "one Error line for the drop")
	st := e.w.Status()
	assert.False(t, st.Healthy)
	assert.NotEmpty(t, st.LastError)

	e.w.PutFlags(wtFlags("drop"), nil, 7)
	e.w.PutSegment("s1", []string{"k"}, nil, 8)
	require.NoError(t, e.w.Start(nil))
	wtWaitUntil(t, "the segment write after the dropped flag change", func() bool {
		return e.w.Status().PendingChanges == 0 && e.client.count() == 2*opts.MaxAttempts+1
	})
	e.stop()

	segments := 0
	for _, c := range e.client.evals() {
		if c.script == SegmentScript {
			segments++
		}
	}
	assert.Equal(t, 1, segments, "the next queued change is still written")
	assert.Equal(t, 2, e.log.errorCount(), "one Error line per dropped change")
}

func TestHealthRecoversAfterSuccess(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	failing := true
	e.client.fail = func(int, wtCall) error {
		if failing {
			return errors.New("redis: connection pool timeout")
		}
		return nil
	}
	data := e.w.keys.FlagData()
	require.ErrorIs(t, e.w.eval("flag write", "flag-script", []string{data}, nil), errDropped)
	require.False(t, e.w.Status().Healthy)

	failing = false
	require.NoError(t, e.w.eval("flag write", "flag-script", []string{data}, nil))
	st := e.w.Status()
	assert.True(t, st.Healthy)
	assert.Empty(t, st.LastError)
}

func TestPermanentRejectionMarksUnhealthy(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(int, wtCall) error { return errors.New("PROXY_BAD_ARGS") }
	err := e.w.eval("flag write", "flag-script", []string{e.w.keys.FlagData()}, nil)
	assert.ErrorIs(t, err, errChunkRejected)
	assert.Equal(t, 1, e.client.count(), "a permanent rejection is not retried")
	st := e.w.Status()
	assert.False(t, st.Healthy)
	assert.NotEmpty(t, st.LastError)
	assert.Equal(t, int64(0), st.FailedRounds, "a refusal is not a failed round")
}

func TestTransientErrorDoesNotIsolateEntities(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 10
	e := newWtEnv(t, opts)
	e.client.fail = func(int, wtCall) error { return errors.New("redis: connection pool timeout") }
	e.w.PutFlags(wtFlags("a", "b", "c"), nil, 7)
	require.NoError(t, e.w.Start(nil))
	wtWaitUntil(t, "the dropped chunk", func() bool {
		return e.w.Status().PendingChanges == 0 && e.client.count() == opts.MaxAttempts
	})
	e.stop()

	for _, c := range e.client.evals() {
		assert.Len(t, wtEntities(c.args), 3, "a transient error never splits the chunk into single entities")
	}
	assert.Equal(t, int64(0), e.w.Status().RejectedChanges, "no entity is refused")
	g := e.w.groups[e.w.keys.FlagCursorField()]
	require.NotNil(t, g, "the flag group is frozen")
	assert.True(t, g.whole, "the dropped change freezes its group until a resync releases it")
}

func TestDropTriggersBoundedResync(t *testing.T) {
	opts := wtOptions()
	// one try per change so retry() records no waits and the recorded waits are only the resync backoff
	opts.MaxAttempts = 1
	e := newWtEnv(t, opts)
	var down atomic.Bool
	var resyncs atomic.Int64
	e.client.fail = func(int, wtCall) error {
		if down.Load() {
			return errors.New("redis: connection pool timeout")
		}
		return nil
	}
	var mu sync.Mutex
	var waits []time.Duration
	fakeWait := e.w.wait
	e.w.wait = func(d time.Duration) bool {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		return fakeWait(d)
	}
	cursorWritten := func(cn int64) bool {
		for _, c := range e.client.evals() {
			if c.script == FlagScript && wtFlagCursor(c.args) == cn {
				return true
			}
		}
		return false
	}

	require.NoError(t, e.w.Start(&wtSource{flags: wtFlags("a"), flagsCn: 7, onFlags: func() { resyncs.Add(1) }}))
	wtWaitUntil(t, "the startup resync", func() bool { return cursorWritten(7) && e.w.Status().PendingChanges == 0 })

	down.Store(true)
	e.w.PutFlags(wtFlags("x"), nil, 8)
	wtWaitUntil(t, "resyncs requested while Redis is down", func() bool { return resyncs.Load() >= 3 })

	down.Store(false)
	e.w.PutFlags(wtFlags("y"), nil, 9)
	wtWaitUntil(t, "the cursor catches up after recovery", func() bool {
		return cursorWritten(9) && e.w.Status().PendingChanges == 0 && e.w.Status().Healthy
	})
	e.stop()

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, waits, "retries and resync requests wait with a backoff")
	for _, d := range waits {
		assert.LessOrEqual(t, d, opts.BackoffMaxWait, "every wait is capped by BackoffMaxWait")
	}
	require.GreaterOrEqual(t, len(waits), 3, "the resync backoff waits at least three times")
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, waits[:3], "the resync backoff doubles from 1s")
}

func TestClassifyBusyTransientNoauthBounded(t *testing.T) {
	t.Run("BUSY wrapped in a Lua error is retried, not skipped", func(t *testing.T) {
		opts := wtOptions()
		e := newWtEnv(t, opts)
		busy := "ERR Error running script (call to f_1a2b): @user_script:1: BUSY Redis is busy running a script."
		e.client.fail = func(n int, _ wtCall) error {
			if n <= 2 {
				return errors.New(busy)
			}
			return nil
		}
		require.NoError(t, e.w.eval("flag write", "flag-script", []string{e.w.keys.FlagData()}, nil))
		assert.Equal(t, 3, e.client.count())
		assert.Empty(t, e.log.errors, "no round failed, so nothing is logged")
	})
	t.Run("NOAUTH is transient, never isolated per entity", func(t *testing.T) {
		assert.Equal(t, errTransient, classify(errors.New("NOAUTH Authentication required")))
	})
}

func TestClassifyPoolTimeoutIsTransient(t *testing.T) {
	// text of go-redis v9 internal/pool ErrPoolTimeout
	assert.Equal(t, errTransient, classify(errors.New("redis: connection pool timeout")))
}

func TestClassifyConnectionErrorsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"EOF", io.EOF},
		{"unexpected EOF", io.ErrUnexpectedEOF},
		{"wrapped EOF", fmt.Errorf("read: %w", io.EOF)},
		{"client closed", errors.New("redis: client is closed")},
		{"broken pipe", fmt.Errorf("write tcp 127.0.0.1:1->127.0.0.1:6379: %w", syscall.EPIPE)},
		{"EPIPE in net.OpError", &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}},
		{"NOAUTH during failover", errors.New("NOAUTH Authentication required")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, errTransient, classify(c.err))
		})
	}
}

func TestClassifyClusterErrorsAndEchoedMarkers(t *testing.T) {
	cases := []struct {
		err  error
		want errClass
	}{
		{errors.New("MOVED 3999 127.0.0.1:6381"), errTransient},
		{errors.New("ASK 3999 127.0.0.1:6381"), errTransient},
		{errors.New("NOREPLICAS Not enough good replicas to write."), errTransient},
		{errors.New("EXECABORT Transaction discarded because of previous errors."), errTransient},
		{errors.New("ERR max number of clients reached"), errTransient},
		{errors.New("redis: connection pool timeout"), errTransient},
		{errors.New("ERR max number of clients reached, try later"), errUnknown},
		{&net.OpError{Op: "dial", Err: errors.New("connection refused")}, errTransient},
		{errors.New("PROXY_BAD_ARGS"), errPermanent},
		{errors.New("PROXY_BAD_ARGS flag OOM"), errPermanent},
		{errors.New("PROXY_BAD_ARGS rule LOADING"), errPermanent},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, classify(c.err), c.err.Error())
	}
}

func TestClassifyTablePinnedToRealErrorStrings(t *testing.T) {
	// Captured from the local Redis at localhost:6379 with EVAL (script errors) and a long script stopped with SCRIPT KILL (BUSY).
	// Rows marked [quoted] come from the Redis and go-redis sources: the local single node cannot produce them.
	cases := []struct {
		name string
		err  string
		want errClass
	}{
		{"direct WRONGTYPE", "WRONGTYPE Operation against a key holding the wrong kind of value", errPermanent},
		{"lua WRONGTYPE", "ERR user_script:1: WRONGTYPE Operation against a key holding the wrong kind of value script: 691ad66b169ddb9e0d5fb480aeb7c7887a2c4b59, on @user_script:1.", errPermanent},
		{"proxy bad args", "ERR PROXY_BAD_ARGS", errPermanent},
		{"proxy meta wrongtype", "ERR user_script:1: PROXY_META_WRONGTYPE script: 0, on @user_script:1.", errTransient},
		{"lua runtime error", "ERR user_script:1: boom script: 82903a0434f1503e152f89c03c9acd881a0e8150, on @user_script:1.", errPermanent},
		{"lua integer error", "ERR value is not an integer or out of range script: da8455f0535fd532821b3713a4eccd80fc4b8457, on @user_script:1.", errPermanent},
		{"lua unknown command", "ERR Unknown Redis command called from script script: ee4002f488cb0514b4fbe5979e61c3bf43204edb, on @user_script:1.", errPermanent},
		{"direct OOM", "OOM command not allowed", errTransient},
		{"lua OOM", "ERR user_script:1: OOM command not allowed when used memory > maxmemory script: 697390339656a7da341b346b26c5ce0552223122, on @user_script:1.", errTransient},
		{"lua READONLY", "ERR user_script:1: READONLY You can't write against a read only replica. script: 0753be4445ca355585f921a9dcf10d124588b3b9, on @user_script:1.", errTransient},
		{"BUSY", "BUSY Redis is busy running a script. You can only call SCRIPT KILL or SHUTDOWN NOSAVE.", errTransient},
		{"[quoted] LOADING", "LOADING Redis is loading the dataset in memory", errTransient},
		{"[quoted] CLUSTERDOWN", "CLUSTERDOWN The cluster is down", errTransient},
		{"[quoted] MOVED", "MOVED 3999 127.0.0.1:6381", errTransient},
		{"[quoted] NOREPLICAS", "NOREPLICAS Not enough good replicas to write.", errTransient},
		{"[quoted] CROSSSLOT", "CROSSSLOT Keys in request don't hash to the same slot", errPermanent},
		{"[quoted] NOAUTH", "NOAUTH Authentication required.", errTransient},
		{"[quoted] MISCONF", "MISCONF Redis is configured to save RDB snapshots, but it's currently unable to persist to disk.", errTransient},
		{"[quoted] WRONGPASS", "WRONGPASS invalid username-password pair or user is disabled.", errUnknown},
		{"[quoted] unlisted", "ERR something Redis has never said", errUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, classify(errors.New(c.err)), c.err)
		})
	}
}

func TestNilSourceMirrorOnly(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags(wtFlags("a"), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the queued change to be written", func() bool { return e.w.Status().PendingChanges == 0 })

	e.w.mtx.Lock()
	canResync := e.w.canResync
	needResync := e.w.needResync
	e.w.mtx.Unlock()
	assert.False(t, canResync, "a nil source is mirror-only")
	assert.False(t, needResync, "no startup resync is scheduled without a source")
	assert.Equal(t, int64(0), e.w.Status().Resyncs)
	e.stop()
}

func TestTypedNilSourceMirrorOnly(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	var typedNil *wtSource
	var src Source = typedNil
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}

	e.w.mtx.Lock()
	canResync := e.w.canResync
	needResync := e.w.needResync
	e.w.mtx.Unlock()
	assert.False(t, canResync, "a typed-nil source behaves as nil")
	assert.False(t, needResync, "no startup resync is scheduled for a typed-nil source")
	assert.Equal(t, int64(0), e.w.Status().Resyncs)
	e.stop()
}

func TestPutAfterStopDoesNotEnqueue(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.Stop()

	done := make(chan struct{})
	go func() {
		e.w.PutFlags(wtFlags("a"), nil, 1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("PutFlags after Stop did not return")
	}
	assert.Zero(t, e.w.Status().PendingChanges, "a change put after Stop is not queued")
	assert.Equal(t, int64(1), e.w.Status().DroppedChanges, "a refused change is counted as dropped")
	assert.Equal(t, 1, strings.Count(e.log.infoText(), wtRefusedAfterStop), "the refusal is logged once")
	e.stop()
}

// wtRefusedAfterStop is the text of the one-time log for a change refused after Stop
const wtRefusedAfterStop = "change refused after Stop"

// TestPutDuringDrainIsRefused: once Stop is called, a Put is refused: it is not queued, it is counted as dropped and
// logged once. The drain still ends when the queue empties.
func TestPutDuringDrainIsRefused(t *testing.T) {
	opts := wtOptions()
	opts.StopTimeoutMs = 10000
	e := newWtEnv(t, opts)
	e.w.PutSegment("s1", []string{"m"}, nil, 1)
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	e.client.fail = func(int, wtCall) error {
		once.Do(func() {
			close(entered)
			wtHold(t, gate, "gate signal")
		})
		return nil
	}
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not start writing")
	}

	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	e.w.PutSegment("late1", []string{"m"}, nil, 1)
	e.w.PutSegment("late2", []string{"m"}, nil, 1)
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return: the drain did not end when the queue emptied")
	}

	var names []string
	for _, c := range e.client.evals() {
		name, _, _, _ := wtSegArgs(c.args)
		names = append(names, name)
	}
	assert.Equal(t, []string{"s1"}, names, "only the change queued before Stop is written")
	assert.Equal(t, int64(2), e.w.Status().DroppedChanges, "both refused changes are counted as dropped")
	assert.Equal(t, 1, strings.Count(e.log.infoText(), wtRefusedAfterStop), "the refusals are logged once")
}

// TestStopDrainsQueueWithinTimeout: Stop keeps the worker draining until the drain deadline (StopTimeoutMs after
// Stop), then drops what is left, counts it and writes no cursor for it.
func TestStopDrainsQueueWithinTimeout(t *testing.T) {
	opts := wtOptions()
	opts.StopTimeoutMs = 1000
	e := newWtEnv(t, opts)
	for _, s := range []string{"s1", "s2", "s3", "s4", "s5"} {
		e.w.PutSegment(s, []string{"m"}, nil, 1)
	}
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	// the first Eval blocks until the test has called Stop; every Eval then takes 400ms of fake time
	e.client.fail = func(int, wtCall) error {
		once.Do(func() {
			close(entered)
			wtHold(t, gate, "gate signal")
		})
		e.clock.Add(400 * time.Millisecond)
		return nil
	}
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not start writing")
	}

	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}

	var names []string
	for _, c := range e.client.evals() {
		name, _, _, _ := wtSegArgs(c.args)
		names = append(names, name)
	}
	assert.Equal(t, []string{"s1", "s2", "s3"}, names, "changes are written while the drain deadline has not passed; s4 is cut off at the deadline and s5 is never sent")
	assert.Contains(t, e.log.infoText(), "2 queued changes dropped", "the cut-off change in flight and the queued s5 are counted")
}

// TestStopReportsLeftoverCount: a healthy Redis takes every queued change before Stop returns, so nothing is left,
// and the log does not promise that a startup resync recovers dropped changes.
func TestStopReportsLeftoverCount(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	for _, s := range []string{"s1", "s2", "s3"} {
		e.w.PutSegment(s, []string{"m"}, nil, 1)
	}
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.Stop()

	assert.Len(t, e.client.evals(), 3, "every queued change is written before the drain deadline")
	info := e.log.infoText()
	assert.Contains(t, info, "0 queued changes dropped", "zero leftovers are reported")
	assert.NotContains(t, strings.ToLower(info), "recover", "the log makes no promise of startup recovery")
	assert.NotContains(t, strings.ToLower(info), "resync", "the log makes no promise of startup recovery")
}

// wtBlockedWait replaces w.wait with the real sleep and signals waiting when the first backoff between attempts (1s) starts
func wtBlockedWait(e *wtEnv, waiting chan<- struct{}) {
	e.w.wait = func(d time.Duration) bool {
		if d >= time.Second {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
		return e.w.sleep(d)
	}
}

// TestStopWakesBackoffForOneImmediateAttempt: Redis is down and the worker sleeps in a 60s backoff. Redis comes back
// one second later and Stop wakes the worker: one immediate attempt succeeds and the queued change is written.
func TestStopWakesBackoffForOneImmediateAttempt(t *testing.T) {
	opts := wtOptions()
	opts.MaxAttempts = 2
	opts.BackoffMaxWait = time.Minute
	opts.StopTimeoutMs = 5000
	e := newWtEnv(t, opts)
	waiting := make(chan struct{}, 1)
	wtBlockedWait(e, waiting)
	var down atomic.Bool
	down.Store(true)
	e.client.fail = func(int, wtCall) error {
		if down.Load() {
			return &net.OpError{Op: "dial", Err: errors.New("down")}
		}
		return nil
	}
	e.w.PutSegment("s1", []string{"m"}, nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not enter the backoff between attempts")
	}

	down.Store(false)
	e.clock.Add(time.Second)
	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}

	assert.Len(t, e.client.evals(), 2, "one failed attempt before the backoff and one immediate attempt after Stop")
	assert.Zero(t, e.w.Status().PendingChanges, "the queued change is written after the wake")
	assert.Contains(t, e.log.infoText(), "0 queued changes dropped", "nothing is left at the deadline")
}

// TestStopDrainDeadlineCapsBackoffWait: Redis is down at Stop and BackoffMaxWait (60s) is far past the drain bound
// (1s). The waits are capped at the drain deadline, the worker exits there, and the changes it still holds are
// dropped and counted.
func TestStopDrainDeadlineCapsBackoffWait(t *testing.T) {
	opts := wtOptions()
	opts.MaxAttempts = 3
	opts.BackoffMaxWait = time.Minute
	opts.StopTimeoutMs = 1000
	e := newWtEnv(t, opts)
	waiting := make(chan struct{}, 1)
	wtBlockedWait(e, waiting)
	e.client.fail = func(int, wtCall) error {
		e.clock.Add(400 * time.Millisecond)
		return &net.OpError{Op: "dial", Err: errors.New("down")}
	}
	e.w.PutSegment("s1", []string{"m"}, nil, 1)
	e.w.PutSegment("s2", []string{"m"}, nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not enter the backoff between attempts")
	}

	start := time.Now()
	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return within 3s: the 60s backoff was not capped at the 1s drain deadline")
	}
	assert.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond, "the waits left before the deadline are paced in real time, not spun through")
	assert.Contains(t, e.log.infoText(), "2 queued changes dropped", "the change in flight and the queued change are counted")
}

// TestStopNotBlockedByHungEvalWriterSide: an Eval that never returns keeps the worker inside the
// call. Stop does not return, the drain deadline passing does not end the worker (the toolkit Eval takes no context,
// so the worker cannot check the deadline mid-call), and once the Eval is released the worker exits and Stop returns.
// The outer wait (stopWaitBound) is tested in package proxy.
func TestStopNotBlockedByHungEvalWriterSide(t *testing.T) {
	opts := wtOptions()
	opts.StopTimeoutMs = 1000
	e := newWtEnv(t, opts)
	e.w.PutSegment("s1", []string{"m"}, nil, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	e.client.fail = func(int, wtCall) error {
		once.Do(func() {
			close(entered)
			wtHold(t, release, "release signal")
		})
		return nil
	}
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWithinDeadline(t, "the worker enters the Eval", entered, 5*time.Second)

	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	// the drain deadline (StopTimeoutMs after Stop) passes while the Eval is still blocked
	e.clock.Add(time.Duration(opts.StopTimeoutMs)*time.Millisecond + time.Second)
	select {
	case <-stopped:
		t.Fatal("Stop returned while the Eval was still blocked")
	case <-e.w.done:
		t.Fatal("the worker exited while the Eval was still blocked")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	wtWithinDeadline(t, "the worker exits after the Eval is released", e.w.done, 5*time.Second)
	wtWithinDeadline(t, "Stop returns after the worker exits", stopped, 5*time.Second)
	assert.Len(t, e.client.evals(), 1, "no Eval is sent once the drain deadline has passed")
}

// wtRejectFlag makes the script refuse the flag entity name with change number cn, as PROXY_BAD_ARGS does
func wtRejectFlag(name string, cn int64) func(int, wtCall) error {
	return func(_ int, c wtCall) error {
		if c.script != FlagScript {
			return nil
		}
		for _, en := range wtEntities(c.args) {
			if en.name == name && en.cn == cn {
				return errors.New("PROXY_BAD_ARGS refused entity " + name)
			}
		}
		return nil
	}
}

func TestSkippedEntityFreezesGroupCursor(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectFlag("bad", 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
	wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE")}, nil, 5)
	wtWaitUntil(t, "the refused flag write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok1", 7, "ACTIVE")}, nil, 7)
	wtWaitUntil(t, "the write after the skip", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, 3)
	assert.Equal(t, int64(3), wtFlagCursor(calls[0].args))
	assert.Equal(t, int64(3), wtFlagCursor(calls[2].args), "the frozen group sends its last written cursor, not 7")
}

func TestFrozenCursorAdvancesAfterCleanWrite(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectFlag("bad", 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
	wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE")}, nil, 5)
	wtWaitUntil(t, "the refused flag write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 6, "ACTIVE")}, nil, 6)
	wtWaitUntil(t, "the clean rewrite of the skipped flag", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok1", 7, "ACTIVE")}, nil, 7)
	wtWaitUntil(t, "the write after the release", func() bool { return e.client.count() == 4 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, 4)
	assert.Equal(t, int64(6), wtFlagCursor(calls[2].args), "the clean write of the skipped entity advances the frozen cursor")
	assert.Equal(t, int64(7), wtFlagCursor(calls[3].args), "a released group sends the change number again")
}

func TestSkippedFlagDoesNotFreezeSegments(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectFlag("bad", 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutSegment("s1", []string{"a"}, nil, 4)
	wtWaitUntil(t, "the first segment write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE")}, nil, 5)
	wtWaitUntil(t, "the refused flag write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.w.PutSegment("s1", []string{"b"}, nil, 9)
	wtWaitUntil(t, "the segment write after the skip", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	calls := e.client.evals()
	require.Len(t, calls, 3)
	_, cur, _, _ := wtSegArgs(calls[2].args)
	assert.Equal(t, int64(9), cur, "a skipped flag does not freeze the segment cursor")
}

func wtRejectAny(rules ...func(int, wtCall) error) func(int, wtCall) error {
	return func(n int, c wtCall) error {
		for _, r := range rules {
			if err := r(n, c); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestRequestResyncReleasesFrozenCursors(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectFlag("bad", 5)
	src := &wtSource{}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the startup resync", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
	wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE")}, nil, 5)
	wtWaitUntil(t, "the refused flag write and its resync", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok1", 7, "ACTIVE")}, nil, 7)
	wtWaitUntil(t, "the write after the skip", func() bool { return e.client.count() == 7 && e.w.Status().PendingChanges == 0 })

	src.flags = []dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE"), wtFlag("ok1", 7, "ACTIVE")}
	src.flagsCn = 9
	e.w.RequestResync()
	wtWaitUntil(t, "the resync that drops the skipped entity", func() bool { return e.client.count() == 9 && e.w.Status().PendingChanges == 0 })
	calls := e.client.evals()
	assert.Equal(t, int64(9), wtFlagCursor(calls[7].args), "a clean resync rewrites the group and releases the frozen cursor")

	src.flags = []dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE"), wtFlag("bad", 5, "ACTIVE"), wtFlag("ok1", 7, "ACTIVE")}
	src.flagsCn = 11
	e.w.RequestResync()
	wtWaitUntil(t, "the resync that re-skips the entity", func() bool { return e.client.count() == 14 && e.w.Status().PendingChanges == 0 })
	calls = e.client.evals()
	assert.Equal(t, int64(9), wtFlagCursor(calls[12].args), "a re-skipped entity keeps the group frozen at the last written cursor")
}

// wtRejectSegKey refuses a segment write that carries the given key, as the script does for inconsistent arguments.
func wtRejectSegKey(key string) func(int, wtCall) error {
	return func(_ int, c wtCall) error {
		if c.script != SegmentScript {
			return nil
		}
		_, _, adds, rems := wtSegArgs(c.args)
		for _, k := range append(adds, rems...) {
			if k.key == key {
				return errors.New("PROXY_BAD_ARGS refused key")
			}
		}
		return nil
	}
}

// TestRejectedSegmentKeyIsNotLogged: a segment key the script refuses is skipped, and the key never appears in a log line.
func TestRejectedSegmentKeyIsNotLogged(t *testing.T) {
	const secretKey = "secret-key-7f3a"
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectSegKey(secretKey)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutSegment("seg", []string{"ok-key", secretKey}, nil, 5)
	wtWaitUntil(t, "the segment write", func() bool { return e.client.count() >= 1 && e.w.Status().PendingChanges == 0 })
	e.stop()

	e.log.mu.Lock()
	msgs := append([]string(nil), e.log.errors...)
	e.log.mu.Unlock()
	require.NotEmpty(t, msgs, "the skipped key is reported")
	for _, m := range msgs {
		assert.NotContains(t, m, secretKey, "a segment key is not logged")
	}
	assert.Equal(t, int64(1), e.w.Status().RejectedChanges, "the refused key is counted as skipped")
}

func TestFreezeReleaseRules(t *testing.T) {
	t.Run("two skipped entities release only when both are clean", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		e.client.fail = wtRejectAny(wtRejectFlag("bad1", 5), wtRejectFlag("bad2", 6))
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
		wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad1", 5, "ACTIVE")}, nil, 5)
		wtWaitUntil(t, "the first refused flag write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad2", 6, "ACTIVE")}, nil, 6)
		wtWaitUntil(t, "the second refused flag write", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad1", 8, "ACTIVE")}, nil, 8)
		wtWaitUntil(t, "the clean write of the first entity", func() bool { return e.client.count() == 4 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok1", 9, "ACTIVE")}, nil, 9)
		wtWaitUntil(t, "the write while the second entity is still skipped", func() bool { return e.client.count() == 5 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad2", 10, "ACTIVE")}, nil, 10)
		wtWaitUntil(t, "the clean write of the second entity", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok2", 11, "ACTIVE")}, nil, 11)
		wtWaitUntil(t, "the write after both releases", func() bool { return e.client.count() == 7 && e.w.Status().PendingChanges == 0 })
		e.stop()

		calls := e.client.evals()
		require.Len(t, calls, 7)
		assert.Equal(t, int64(3), wtFlagCursor(calls[3].args), "the group stays frozen while bad2 is still skipped")
		assert.Equal(t, int64(3), wtFlagCursor(calls[4].args), "the group stays frozen while bad2 is still skipped")
		assert.Equal(t, int64(10), wtFlagCursor(calls[5].args), "the clean write of bad2 is the last skipped entity, so the cursor advances")
		assert.Equal(t, int64(11), wtFlagCursor(calls[6].args), "a released group sends the change number again")
	})

	t.Run("skipped-set cap falls back to a whole-group freeze", func(t *testing.T) {
		o := wtOptions()
		o.MaxPendingChanges = 2
		e := newWtEnv(t, o)
		e.client.fail = wtRejectAny(wtRejectFlag("bad1", 5), wtRejectFlag("bad2", 6), wtRejectFlag("bad3", 7))
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
		wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
		for i, name := range []string{"bad1", "bad2", "bad3"} {
			e.w.PutFlags([]dtos.SplitDTO{wtFlag(name, int64(5+i), "ACTIVE")}, nil, int64(5+i))
			wtWaitUntil(t, "the refused flag write "+name, func() bool { return e.client.count() == 2+i && e.w.Status().PendingChanges == 0 })
		}
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad1", 8, "ACTIVE")}, nil, 8)
		wtWaitUntil(t, "the clean write of bad1", func() bool { return e.client.count() == 5 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad2", 9, "ACTIVE")}, nil, 9)
		wtWaitUntil(t, "the clean write of bad2", func() bool { return e.client.count() == 6 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad3", 10, "ACTIVE")}, nil, 10)
		wtWaitUntil(t, "the clean write of bad3", func() bool { return e.client.count() == 7 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok2", 11, "ACTIVE")}, nil, 11)
		wtWaitUntil(t, "the write after the clean rewrites", func() bool { return e.client.count() == 8 && e.w.Status().PendingChanges == 0 })
		e.stop()

		calls := e.client.evals()
		require.Len(t, calls, 8)
		assert.Equal(t, int64(3), wtFlagCursor(calls[7].args), "after the cap the whole group stays frozen even when every named entity is clean")
	})

	t.Run("a marshal failure counts as a skip", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		orig := e.w.marshal
		e.w.marshal = func(v interface{}) ([]byte, error) {
			if d, ok := v.(dtos.SplitDTO); ok && d.Name == "bad" {
				return nil, errors.New("cannot marshal")
			}
			return orig(v)
		}
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
		wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE"), wtFlag("ok1", 7, "ACTIVE")}, nil, 7)
		wtWaitUntil(t, "the write with the marshal failure", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
		e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok2", 9, "ACTIVE")}, nil, 9)
		wtWaitUntil(t, "the write after the marshal failure", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
		e.stop()

		calls := e.client.evals()
		require.Len(t, calls, 3)
		assert.Equal(t, int64(3), wtFlagCursor(calls[1].args), "the write that skipped an entity sends the last written cursor")
		assert.Equal(t, int64(3), wtFlagCursor(calls[2].args), "the group stays frozen after a marshal failure, whose entity name stays pending until it is written cleanly")
	})
}

func TestMirrorOnlyRestartAcceptedGap(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = wtRejectFlag("bad", 5)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("ok0", 3, "ACTIVE")}, nil, 3)
	wtWaitUntil(t, "the first flag write", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("bad", 5, "ACTIVE")}, nil, 5)
	wtWaitUntil(t, "the refused flag write", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.stop()

	restarted, err := NewWriter(e.client, wtKeys(t), wtFingerprint, wtOptions(), e.log)
	require.NoError(t, err)
	t.Cleanup(restarted.Stop)
	if err := restarted.Start(nil); err != nil {
		t.Fatal(err)
	}
	restarted.PutFlags([]dtos.SplitDTO{wtFlag("ok1", 7, "ACTIVE")}, nil, 7)
	wtWaitUntil(t, "the first write after restart", func() bool { return e.client.count() == 3 && restarted.Status().PendingChanges == 0 })

	calls := e.client.evals()
	require.Len(t, calls, 3)
	assert.Equal(t, int64(7), wtFlagCursor(calls[2].args), "accepted limitation (OI23): the freeze is lost on a mirror-only restart, so the first write advances the cursor past the skipped entity")
}

func TestPurgeLoopHonorsStop(t *testing.T) {
	c := &wtClient{meta: map[string]string{}}
	opts := wtOptions()
	opts.PurgeBatch = 2
	w, err := NewWriter(c, wtKeys(t), wtFingerprint, opts, &wtLogger{})
	require.NoError(t, err)
	c.purgeFail = func(n int, call wtCall) error {
		if call.script != PurgeScript {
			return nil
		}
		if n == 3 {
			w.Stop()
		}
		c.meta[purgeLastField] = "2"
		if n > 50 {
			c.meta[purgeLastField] = "0"
		}
		return nil
	}

	err = w.purge(map[string]string{"seg.s1.till": "1"})

	assert.ErrorIs(t, err, errStopped)
	purges := len(c.purgeEvals())
	assert.Equal(t, 3, purges, "the loop checks stopped between PurgeScript calls and stops after the call that saw the stop")
}

// TestStopWithPendingResync: Stop during a running resync with another resync pending and changes queued.
// The running resync stops at its next Eval, the pending one is skipped, and the queue is written first.
func TestStopWithPendingResync(t *testing.T) {
	opts := wtOptions()
	opts.StopTimeoutMs = 1000
	e := newWtEnv(t, opts)
	src := &wtSource{
		flags:   wtFlags("f1"),
		flagsCn: 5,
		rbs:     []dtos.RuleBasedSegmentDTO{{Name: "rb1", ChangeNumber: 5}},
		rbsCn:   5,
	}
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	// the first Eval is the running resync's flag write; it blocks until the test has queued work and called Stop
	e.client.fail = func(int, wtCall) error {
		once.Do(func() {
			close(entered)
			wtHold(t, gate, "gate signal")
		})
		e.clock.Add(100 * time.Millisecond)
		return nil
	}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the resync did not start")
	}
	e.w.RequestResync()
	for _, s := range []string{"s1", "s2", "s3"} {
		e.w.PutSegment(s, []string{"m"}, nil, 1)
	}

	stopped := make(chan struct{})
	go func() {
		e.w.Stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e.w.stopped)
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}

	calls := e.client.evals()
	require.Len(t, calls, 4, "the flag write, then the three queued segments: the running resync does not reach its rule-based write and the pending resync does not run")
	var names []string
	for _, c := range calls {
		if c.script == SegmentScript {
			name, _, _, _ := wtSegArgs(c.args)
			names = append(names, name)
		}
	}
	assert.Equal(t, []string{"s1", "s2", "s3"}, names, "the queue is served first")
	assert.Equal(t, 1, src.flagsCalls, "the pending resync is skipped, so the source is read by one resync only")
	assert.Contains(t, e.log.infoText(), "pending resync skipped during the shutdown drain")
	assert.Contains(t, e.log.infoText(), "resync cut short by Stop")
	assert.Contains(t, e.log.infoText(), "0 queued changes dropped")
}

// TestPurgeRunsAfterResyncAndAbortsOnDrain: the startup purge runs after the first resync, and with many
// segments it aborts between PurgeScript calls when draining starts.
func TestPurgeRunsAfterResyncAndAbortsOnDrain(t *testing.T) {
	opts := wtOptions()
	opts.PurgeBatch = 1
	opts.StopTimeoutMs = 1000

	// purge runs after the resync: the first PurgeScript call must see every resync write already made
	e := newWtEnv(t, opts)
	e.client.meta = map[string]string{}
	for i := 1; i <= 5; i++ {
		e.client.meta[e.w.keys.SegmentCursorField(fmt.Sprintf("seg%d", i))] = "1"
	}
	var resyncCallsAtPurge int
	e.client.purgeFail = func(n int, _ wtCall) error {
		if n == 1 {
			e.client.mu.Lock()
			resyncCallsAtPurge = len(e.client.calls)
			e.client.mu.Unlock()
		}
		return nil
	}
	if err := e.w.Start(&wtSource{flags: wtFlags("f1"), flagsCn: 5}); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "seven purge targets", func() bool { return len(e.client.purgeEvals()) == 7 })
	e.w.Stop()
	require.Len(t, e.client.purgeEvals(), 7, "flags, rule-based segments and five segments named in meta")
	assert.Greater(t, resyncCallsAtPurge, 0, "the first resync write happened before the first purge call")
	assert.Equal(t, resyncCallsAtPurge, len(e.client.evals()), "no resync write follows the purge")

	// with many segments the purge pass aborts between PurgeScript calls once draining starts
	e2 := newWtEnv(t, opts)
	e2.client.meta = map[string]string{}
	for i := 1; i <= 5; i++ {
		e2.client.meta[e2.w.keys.SegmentCursorField(fmt.Sprintf("seg%d", i))] = "1"
	}
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	// every purge call reports a full batch so the pass keeps going; the second call blocks until Stop has been called
	e2.client.purgeFail = func(n int, _ wtCall) error {
		if n == 2 {
			once.Do(func() {
				close(entered)
				wtHold(t, gate, "gate signal")
			})
		}
		e2.client.mu.Lock()
		e2.client.meta[purgeLastField] = "1"
		e2.client.mu.Unlock()
		return nil
	}
	if err := e2.w.Start(&wtSource{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the purge pass did not start")
	}
	stopped := make(chan struct{})
	go func() {
		e2.w.Stop()
		close(stopped)
	}()
	wtWaitUntil(t, "stop requested", e2.w.stopped)
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
	assert.Len(t, e2.client.purgeEvals(), 2, "the pass aborts at the next PurgeScript call after the drain starts")
	assert.Contains(t, e2.log.infoText(), "startup expiry purge cut short by Stop")
}

// wtQueued returns a copy of the queue (the worker is not started in these tests)
func wtQueued(w *Writer) []*change {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return append([]*change(nil), w.queue...)
}

func wtQueuedSize(w *Writer) int {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.queuedSize
}

func TestEnqueueCoalescesFlagChanges(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	t0 := e.clock.Now()
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("a", 5, statusActive), wtFlag("b", 5, statusActive)}, nil, 5)
	e.clock.Add(time.Second)
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("b", 4, statusActive), wtFlag("c", 6, statusActive)}, []dtos.SplitDTO{wtFlag("a", 6, statusArchived)}, 6)
	e.clock.Add(time.Second)
	e.w.PutFlags(nil, []dtos.SplitDTO{wtFlag("b", 5, statusArchived)}, 5)
	e.w.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{{Name: "r1", ChangeNumber: 2, Status: statusActive}}, nil, 2)
	e.w.PutRuleBasedSegments(nil, []dtos.RuleBasedSegmentDTO{{Name: "r1", ChangeNumber: 9, Status: statusArchived}}, 9)

	q := wtQueued(e.w)
	require.Len(t, q, 2, "one queued change per group: flags and rule-based segments")
	flags := q[0]
	require.Equal(t, kindFlags, flags.kind)
	cn := map[string]int64{}
	removed := map[string]bool{}
	for i, d := range flags.flags {
		cn[d.Name] = d.ChangeNumber
		removed[d.Name] = d.Status != statusActive || flags.removals[i]
	}
	assert.Equal(t, map[string]int64{"a": 6, "b": 5, "c": 6}, cn, "per name the higher change number wins, cn is kept per entry")
	assert.Equal(t, map[string]bool{"a": true, "b": true, "c": false}, removed, "equal cn: the later arrival wins; removed entries stay")
	assert.Equal(t, int64(6), flags.cn, "the change number of the merged change is the max")
	assert.Equal(t, 3, flags.size, "queuedSize counts distinct merged entities")
	assert.Equal(t, t0, flags.enqueuedAt, "the merged change keeps the oldest enqueue time")

	rbs := q[1]
	require.Equal(t, kindRuleBasedSegments, rbs.kind)
	require.Len(t, rbs.rbs, 1)
	assert.Equal(t, int64(9), rbs.rbs[0].ChangeNumber)
	assert.Equal(t, statusArchived, rbs.rbs[0].Status)
	assert.Equal(t, 4, wtQueuedSize(e.w), "queuedSize is the merged distinct size: 3 flags + 1 rule-based segment")
}

func TestEnqueueCoalescesSegmentChanges(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutSegment("s", []string{"k1", "k2"}, []string{"k3"}, 5)
	e.w.PutSegment("s", []string{"k3", "k4"}, []string{"k4"}, 5)
	e.w.PutSegment("s", []string{"k2"}, nil, 4)
	e.w.PutSegment("s", nil, []string{"k2"}, 6)
	e.w.PutSegment("other", []string{"k1"}, nil, 6)

	q := wtQueued(e.w)
	require.Len(t, q, 2, "a different segment name is never merged")
	s := q[0]
	require.Equal(t, "s", s.segment)
	adds := map[string]int64{}
	for _, k := range s.adds {
		adds[k.key] = k.cn
	}
	rems := map[string]int64{}
	for _, k := range s.rems {
		rems[k.key] = k.cn
	}
	assert.Len(t, s.adds, 1, "no key is listed twice")
	assert.Equal(t, map[string]int64{"k1": 5}, adds, "equal cn: removal wins; k3 and k4 stay removed, and the add of k4 in the same change does not replace it")
	assert.Equal(t, map[string]int64{"k2": 6, "k3": 5, "k4": 5}, rems, "higher cn removes k2; the lower cn add at 4 is ignored")
	assert.Equal(t, int64(6), s.cn, "the change number of the merged change is the max")
	assert.Equal(t, 4, s.size, "size is the merged distinct key count")
	assert.Equal(t, 5, wtQueuedSize(e.w), "queuedSize is 4 merged keys plus 1 key of the other segment")
}

// At an equal change number the removal wins whatever the arrival order, the same rule as the Lua script
func TestEnqueueSegmentTieRemovalWinsBothOrders(t *testing.T) {
	cases := map[string]func(w *Writer){
		"add then removal": func(w *Writer) {
			w.PutSegment("s", []string{"k"}, nil, 5)
			w.PutSegment("s", nil, []string{"k"}, 5)
		},
		"removal then add": func(w *Writer) {
			w.PutSegment("s", nil, []string{"k"}, 5)
			w.PutSegment("s", []string{"k"}, nil, 5)
		},
	}
	for name, put := range cases {
		t.Run(name, func(t *testing.T) {
			e := newWtEnv(t, wtOptions())
			put(e.w)
			q := wtQueued(e.w)
			require.Len(t, q, 1)
			assert.Empty(t, q[0].adds, "the add at the tied change number is not kept")
			require.Len(t, q[0].rems, 1)
			assert.Equal(t, segmentKey{key: "k", cn: 5}, q[0].rems[0], "the removal wins at the tied change number")
			assert.Equal(t, 1, q[0].size)
		})
	}
}

func TestCoalesceSkipsInflightAndKeepsGroupOrder(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags(wtFlags("a"), nil, 1)
	e.w.PutSegment("s1", []string{"k1"}, nil, 1)
	e.w.PutFlags(wtFlags("b"), nil, 2)

	q := wtQueued(e.w)
	require.Len(t, q, 2, "the second flag change merges into the first; the segment change stays second")
	assert.Equal(t, kindFlags, q[0].kind)
	assert.Equal(t, kindSegment, q[1].kind)
	assert.Len(t, q[0].flags, 2)

	job, ok := e.w.next()
	require.True(t, ok)
	require.Equal(t, kindFlags, job.change.kind)
	require.Len(t, job.change.flags, 2)

	e.w.PutFlags(wtFlags("c"), nil, 3)
	q = wtQueued(e.w)
	require.Len(t, q, 2)
	assert.Equal(t, kindSegment, q[0].kind)
	assert.Equal(t, kindFlags, q[1].kind, "a flag change is not merged into the in-flight change: it is appended")
	assert.Len(t, job.change.flags, 2, "the in-flight change is unchanged")

	job, ok = e.w.next()
	require.True(t, ok)
	require.Equal(t, kindSegment, job.change.kind)
	e.w.PutSegment("s1", []string{"k2"}, nil, 4)
	q = wtQueued(e.w)
	require.Len(t, q, 2, "the popped segment left the group index: the new segment change is appended")
	assert.Len(t, q[1].adds, 1)

	e.w.enterMismatch()
	require.NoError(t, e.w.Reset())
	assert.Empty(t, wtQueued(e.w), "the clear empties the queue")
	e.w.PutFlags(wtFlags("d"), nil, 5)
	q = wtQueued(e.w)
	require.Len(t, q, 1, "the clear dropped the group index: no merge into a removed change")
	assert.Len(t, q[0].flags, 1)
}

func wtNamedFlags(prefix string, n int, cn int64) []dtos.SplitDTO {
	out := make([]dtos.SplitDTO, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, wtFlag(fmt.Sprintf("%s%d", prefix, i), cn, statusActive))
	}
	return out
}

func wtOverflowed(w *Writer) bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.overflow
}

func TestCoalesceOutOfOrderCnKeepsHigherCn(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags(nil, []dtos.SplitDTO{wtFlag("a", 7, statusArchived)}, 7)
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("a", 5, statusActive)}, nil, 5)
	e.w.PutSegment("s", nil, []string{"k"}, 7)
	e.w.PutSegment("s", []string{"k"}, nil, 5)

	q := wtQueued(e.w)
	require.Len(t, q, 2)
	flags := q[0]
	require.Len(t, flags.flags, 1)
	assert.Equal(t, int64(7), flags.flags[0].ChangeNumber, "the higher cn entry is kept; the later lower cn arrival does not replace it")
	assert.Equal(t, statusArchived, flags.flags[0].Status)
	assert.True(t, flags.removals[0])
	assert.Equal(t, int64(7), flags.cn, "the change number of the merged change is the max")

	seg := q[1]
	assert.Empty(t, seg.adds, "a lower cn add does not undo a higher cn removal")
	assert.Equal(t, []segmentKey{{key: "k", cn: 7}}, seg.rems)
	assert.Equal(t, int64(7), seg.cn)
}

func TestOverflowTestedAfterMerge(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 1000
	e := newWtEnv(t, opts)
	e.w.PutFlags(wtNamedFlags("a", 600, 1), nil, 1)
	e.w.PutFlags(wtNamedFlags("a", 500, 2), nil, 2)

	assert.False(t, wtOverflowed(e.w), "re-updates of queued names merge and never overflow")
	assert.Equal(t, 600, wtQueuedSize(e.w), "queuedSize counts the distinct merged names")
	assert.Len(t, wtQueued(e.w), 1)
	assert.Equal(t, int64(0), e.w.Status().Resyncs)

	e.w.PutFlags(wtNamedFlags("b", 500, 3), nil, 3)

	assert.True(t, wtOverflowed(e.w), "a genuinely new set of names overflows")
	q := wtQueued(e.w)
	require.Len(t, q, 1, "the triggering change is enqueued after the clear")
	assert.Len(t, q[0].flags, 500)
}

func TestCoalesceCostIndependentOfQueuedChangeSize(t *testing.T) {
	ten := wtKeysOf(10, "k")
	measure := func(n int) float64 {
		e := newWtEnv(t, wtOptions())
		e.w.PutSegment("s", wtKeysOf(n, "k"), nil, 1)
		cn := int64(2)
		e.w.PutSegment("s", ten, nil, cn)
		queued := wtQueued(e.w)
		require.Len(t, queued, 1)
		ptr := reflect.ValueOf(queued[0].pos).Pointer()
		allocs := testing.AllocsPerRun(50, func() {
			cn++
			e.w.PutSegment("s", ten, nil, cn)
		})
		assert.Equal(t, ptr, reflect.ValueOf(wtQueued(e.w)[0].pos).Pointer(), "the name map is built once and reused by later merges")
		assert.Equal(t, n, len(wtQueued(e.w)[0].pos), "the map indexes every queued key")
		return allocs
	}
	small := measure(1000)
	big := measure(1000000)
	assert.InDelta(t, small, big, 0.5, "allocations of a 10-entity merge do not depend on the queued size")
}

func TestMergeIntoLargeQueuedChangeUsesNameIndex(t *testing.T) {
	const n = 1000000
	perOp := func(size int) (time.Duration, *wtEnv) {
		e := newWtEnv(t, wtOptions())
		e.w.PutSegment("s", wtKeysOf(size, "k"), nil, 1)
		e.w.PutSegment("s", []string{"warm"}, nil, 2)
		best := time.Duration(1 << 62)
		for r := 0; r < 3; r++ {
			start := time.Now()
			for i := 0; i < 1000; i++ {
				e.w.PutSegment("s", []string{fmt.Sprintf("new-%d-%d", r, i)}, nil, 2)
				if el := time.Since(start); el > 10*time.Second {
					t.Fatalf("single-name merge %d of round %d took over 10s with %d names queued: the merge rescans the change", i, r, size)
				}
			}
			best = min(best, time.Since(start))
		}
		return best, e
	}
	small, _ := perOp(1000)
	big, e := perOp(n)
	assert.LessOrEqual(t, float64(big), 5*float64(small), "per-op time of single-name merges must not grow with the queued size")

	e.w.PutSegment("s", nil, []string{"k5"}, 3)
	q := wtQueued(e.w)
	require.Len(t, q, 1)
	assert.Len(t, q[0].adds, n+1+3000-1, "1M initial keys, 1 warm key, 3000 new keys, minus k5")
	assert.Equal(t, []segmentKey{{key: "k5", cn: 3}}, q[0].rems)
	assert.Equal(t, int64(3), q[0].cn)
}

func TestOverflowMergedTriggerKept(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 1000
	e := newWtEnv(t, opts)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var enter sync.Once
	src := &wtSource{onFlags: func() {
		enter.Do(func() {
			close(entered)
			wtHold(t, release, "release signal")
		})
	}}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the startup resync never started")
	}

	e.w.PutFlags(wtNamedFlags("a", 600, 1), nil, 1)
	e.w.PutFlags(wtNamedFlags("b", 500, 2), nil, 2)

	assert.True(t, wtOverflowed(e.w), "600 queued names plus 500 new names exceed the cap")
	assert.Equal(t, int64(1), e.w.Status().Resyncs, "a source is configured: the overflow schedules and counts one resync")
	q := wtQueued(e.w)
	if assert.Len(t, q, 1, "the queue is cleared, then the triggering change is enqueued") {
		assert.Len(t, q[0].flags, 500, "the original unmerged trigger is kept, not merged into the dropped 600")
		assert.Equal(t, "b0", q[0].flags[0].Name)
		assert.Equal(t, 500, q[0].size)
	}
	assert.Equal(t, 500, wtQueuedSize(e.w), "queuedSize counts only the kept trigger")
	releaseOnce.Do(func() { close(release) })
	e.stop()
}

func TestMismatchResetClearsGroupIndex(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.w.PutFlags(wtFlags("a"), nil, 1)
	e.w.PutFlags(wtFlags("b"), nil, 2)
	require.Len(t, wtQueued(e.w), 1)

	e.w.enterMismatch()
	require.NoError(t, e.w.Reset())
	assert.Empty(t, wtQueued(e.w), "mismatch and Reset empty the queue")
	assert.Equal(t, 0, wtQueuedSize(e.w))

	e.w.PutFlags(wtFlags("c"), nil, 3)
	q := wtQueued(e.w)
	if assert.Len(t, q, 1, "the group index is empty: c is a fresh slot, not merged into a removed change") {
		assert.Equal(t, "c", q[0].flags[0].Name)
	}
	assert.Equal(t, 1, wtQueuedSize(e.w), "queuedSize equals the queue contents")

	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the write of c", func() bool { return e.client.count() == 1 && e.w.Status().PendingChanges == 0 })
	e.stop()
	calls := e.client.evals()
	if assert.Len(t, calls, 1) {
		ents := wtEntities(calls[0].args)
		if assert.Len(t, ents, 1) {
			assert.Equal(t, "c", ents[0].name)
		}
	}
}

func TestCoalescedChangeCursorAndChunks(t *testing.T) {
	opts := wtOptions()
	opts.ChunkSize = 2

	e := newWtEnv(t, opts)
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("a", 1, statusActive), wtFlag("b", 1, statusActive)}, nil, 1)
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("c", 2, statusActive)}, nil, 2)
	e.w.PutFlags([]dtos.SplitDTO{wtFlag("a", 5, statusActive)}, nil, 5)
	require.Len(t, wtQueued(e.w), 1, "one merged change")
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the merged change is written", func() bool { return e.client.count() == 2 && e.w.Status().PendingChanges == 0 })
	e.stop()
	calls := e.client.evals()
	if assert.Len(t, calls, 2, "three entities in chunks of two") {
		assert.Equal(t, noCursor, wtFlagCursor(calls[0].args), "only the last chunk carries the cursor")
		first := wtEntities(calls[0].args)
		if assert.Len(t, first, 2) {
			assert.Equal(t, int64(5), first[0].cn, "the merged entity carries its newest cn")
		}
		assert.Equal(t, int64(5), wtFlagCursor(calls[1].args), "the last chunk sends the newest cn of the merged change")
		last := wtEntities(calls[1].args)
		if assert.Len(t, last, 1) {
			assert.Equal(t, "c", last[0].name)
		}
	}

	skip := newWtEnv(t, opts)
	skip.w.PutFlags([]dtos.SplitDTO{wtFlag("s", 4, statusActive)}, nil, 4)
	skip.client.fail = wtRejectFlag("x", 6)
	if err := skip.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the first change is written", func() bool { return skip.client.count() == 1 && skip.w.Status().PendingChanges == 0 })
	skip.w.PutFlags([]dtos.SplitDTO{wtFlag("x", 6, statusActive), wtFlag("y", 7, statusActive)}, nil, 7)
	wtWaitUntil(t, "x is skipped and y is written", func() bool { return skip.client.count() == 4 && skip.w.Status().PendingChanges == 0 })
	skip.stop()
	skipCalls := skip.client.evals()
	last := skipCalls[len(skipCalls)-1]
	assert.Equal(t, int64(4), wtFlagCursor(last.args), "with x skipped the group sends the last written cursor, not 7")
	if ents := wtEntities(last.args); assert.Len(t, ents, 1) {
		assert.Equal(t, "y", ents[0].name)
	}

	replay := newWtEnv(t, opts)
	replay.w.PutFlags([]dtos.SplitDTO{wtFlag("a", 3, statusActive), wtFlag("b", 3, statusActive)}, nil, 3)
	replay.w.PutFlags([]dtos.SplitDTO{wtFlag("c", 2, statusActive)}, nil, 2)
	replay.client.fail = func(n int, _ wtCall) error {
		if n == 2 {
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return nil
	}
	if err := replay.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the replayed chunk is written", func() bool { return replay.client.count() == 3 && replay.w.Status().PendingChanges == 0 })
	replay.stop()
	rc := replay.client.evals()
	if assert.Len(t, rc, 3, "the refused send is retried once; only the failed chunk is replayed") {
		assert.Equal(t, noCursor, wtFlagCursor(rc[0].args), "the first chunk of the merged change carries no cursor")
		assert.Equal(t, rc[1].args, rc[2].args, "the replayed last chunk is identical: idempotent on replay")
		assert.Equal(t, int64(3), wtFlagCursor(rc[2].args))
	}
}

func TestOutageQueueBoundedByDistinctEntities(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 50
	e := newWtEnv(t, opts)
	var down atomic.Bool
	down.Store(true)
	e.client.fail = func(int, wtCall) error {
		if down.Load() {
			return &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		return nil
	}
	e.w.wait = func(time.Duration) bool {
		time.Sleep(time.Millisecond)
		return !e.w.stopped()
	}
	one := func(i int) []dtos.SplitDTO {
		return []dtos.SplitDTO{wtFlag(fmt.Sprintf("f%d", i%10), int64(i+1), statusActive)}
	}

	e.w.PutFlags(one(0), nil, 1)
	if err := e.w.Start(nil); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the first change is in flight", func() bool { return e.client.count() >= 1 })
	// each change is dropped after MaxAttempts, so the queue keeps moving while changes arrive; its size stays
	// within the 10 distinct flags however many changes arrive (re-updates of a queued name merge)
	maxQueued := 0
	for i := 1; i < 1000; i++ {
		e.w.PutFlags(one(i), nil, int64(i+1))
		if q := wtQueuedSize(e.w); q > maxQueued {
			maxQueued = q
		}
	}
	assert.LessOrEqual(t, maxQueued, 10, "the queue never holds more entities than the 10 distinct flags during the outage")
	assert.False(t, wtOverflowed(e.w), "re-updates of queued names merge: no overflow during the outage")
	assert.Equal(t, int64(0), e.w.Status().Resyncs)
	wtWaitUntil(t, "a change is dropped after MaxAttempts", func() bool { return e.w.Status().FailedRounds >= 1 })

	// the outage ends; a later change carries all 10 flags with new change numbers, so the final state is written by it
	down.Store(false)
	final := make([]dtos.SplitDTO, 0, 10)
	for k := 0; k < 10; k++ {
		final = append(final, wtFlag(fmt.Sprintf("f%d", k), int64(2000+k), statusActive))
	}
	e.w.PutFlags(final, nil, 2010)
	wtWaitUntil(t, "the final change is written after the outage", func() bool {
		return e.w.Status().Healthy && e.w.Status().PendingChanges == 0
	})
	e.stop()

	calls := e.client.evals()
	last := calls[len(calls)-1]
	got := map[string]int64{}
	for _, en := range wtEntities(last.args) {
		got[en.name] = en.cn
	}
	want := map[string]int64{}
	for k := 0; k < 10; k++ {
		want[fmt.Sprintf("f%d", k)] = int64(2000 + k)
	}
	// the group stays frozen after the drops and Start(nil) makes the resync a no-op, so the cursor is not sent
	assert.Equal(t, want, got, "the last write holds the final change for every flag")
	assert.Equal(t, int64(-1), wtFlagCursor(last.args), "the frozen group sends no real cursor without a resync")
}

// wtWithinDeadline fails the test unless ch is closed within d
func wtWithinDeadline(t *testing.T, what string, ch <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// TestWorkerPanicIsRecoveredAndReported: a panic in the worker run loop, in a resync pass and in a Source
// callback is recovered: one Error line with a stack, Healthy false, LastError set, Put* does not panic and Stop returns.
func TestWorkerPanicIsRecoveredAndReported(t *testing.T) {
	const deadline = 5 * time.Second
	cases := []struct {
		name  string
		panic string
		setup func(e *wtEnv) Source
	}{
		{
			name:  "worker run loop",
			panic: "eval panic payload",
			setup: func(e *wtEnv) Source {
				e.client.fail = func(int, wtCall) error { panic("eval panic payload") }
				return nil
			},
		},
		{
			name:  "resync pass",
			panic: "resync panic payload",
			setup: func(e *wtEnv) Source {
				e.client.fail = func(int, wtCall) error { panic("resync panic payload") }
				return &wtSource{segs: []SegmentState{{Name: "s", Active: map[string]int64{"k": 1}, ChangeNumber: 1}}}
			},
		},
		{
			name:  "source callback",
			panic: "source panic payload",
			setup: func(e *wtEnv) Source {
				return &wtSource{onFlags: func() { panic("source panic payload") }}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newWtEnv(t, wtOptions())
			src := tc.setup(e)
			if err := e.w.Start(src); err != nil {
				t.Fatal(err)
			}
			e.w.PutFlags(wtFlags("a"), nil, 1)

			wtWithinDeadline(t, "the worker to exit after the panic", e.w.done, deadline)

			e.log.mu.Lock()
			errs := append([]string(nil), e.log.errors...)
			e.log.mu.Unlock()
			require.Len(t, errs, 1, "exactly one Error line")
			assert.Contains(t, errs[0], "goroutine ", "the Error line carries a stack")
			assert.NotContains(t, errs[0], tc.panic, "the Error line carries no panic payload")

			st := e.w.Status()
			assert.False(t, st.Healthy, "Status.Healthy is false after the panic")
			assert.NotEmpty(t, st.LastError, "Status.LastError is set")
			assert.NotContains(t, st.LastError, tc.panic, "LastError carries no payload")

			queuedBefore := len(wtQueued(e.w))
			require.NotPanics(t, func() {
				e.w.PutFlags(wtFlags("b"), nil, 2)
				e.w.PutSegment("s", []string{"k"}, nil, 3)
				e.w.PutRuleBasedSegments(nil, nil, 4)
			}, "Put* after the worker died does not panic")
			assert.Len(t, wtQueued(e.w), queuedBefore, "Put* after the worker died queues nothing")

			stopped := make(chan struct{})
			go func() {
				e.w.Stop()
				close(stopped)
			}()
			wtWithinDeadline(t, "Stop after the worker died", stopped, deadline)
		})
	}
}

// TestStopTwiceAndConcurrentWithStart: Stop twice, Stop without a successful Start and Stop
// concurrent with Start all return, and once Stop returns the worker has exited.
func TestStopTwiceAndConcurrentWithStart(t *testing.T) {
	const deadline = 5 * time.Second

	t.Run("stop twice", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			ret := make(chan struct{})
			go func() {
				e.w.Stop()
				close(ret)
			}()
			wtWithinDeadline(t, fmt.Sprintf("Stop call %d to return", i+1), ret, deadline)
		}
		select {
		case <-e.w.done:
		default:
			t.Fatal("the worker has not exited after Stop returned")
		}
	})

	t.Run("stop without a started worker", func(t *testing.T) {
		e := newWtEnv(t, wtOptions())
		for i := 0; i < 2; i++ {
			ret := make(chan struct{})
			go func() {
				e.w.Stop()
				close(ret)
			}()
			wtWithinDeadline(t, fmt.Sprintf("Stop call %d without Start to return", i+1), ret, deadline)
		}
		if err := e.w.Start(nil); err != nil {
			t.Fatal(err)
		}
		e.w.mtx.Lock()
		started := e.w.started
		e.w.mtx.Unlock()
		assert.False(t, started, "Start after Stop does not launch a worker")
	})

	t.Run("stop concurrent with start", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			e := newWtEnv(t, wtOptions())
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				assert.NoError(t, e.w.Start(nil))
			}()
			go func() {
				defer wg.Done()
				e.w.Stop()
			}()
			joined := make(chan struct{})
			go func() {
				wg.Wait()
				close(joined)
			}()
			wtWithinDeadline(t, "Start and Stop to return", joined, deadline)

			e.w.mtx.Lock()
			started := e.w.started
			e.w.mtx.Unlock()
			if started {
				select {
				case <-e.w.done:
				default:
					t.Fatalf("iteration %d: the worker still runs after Stop returned", i)
				}
			}
		}
	})
}

// redisReply is a Redis error reply as the client returns it (it implements the RedisError marker)
type redisReply string

func (r redisReply) Error() string { return string(r) }
func (redisReply) RedisError()     {}

// a probe that fails with a script error means the server is too old for TIME-then-write scripts
func TestStartProbeFailsOnScriptErrorWithMinimumVersion(t *testing.T) {
	t.Run("script error", func(t *testing.T) {
		c := &wtClient{probeFail: func() error {
			return redisReply("ERR Write commands not allowed after non deterministic commands")
		}}
		opts := wtOptions()
		log := &wtLogger{}
		w, err := NewWriter(c, wtKeys(t), wtFingerprint, opts, log)
		require.NoError(t, err)

		startErr := w.Start(nil)
		var initErr *common.InitializationError
		require.ErrorAs(t, startErr, &initErr)
		assert.Equal(t, common.ExitRedisInitializationFailed, initErr.ExitCode())
		assert.Contains(t, startErr.Error(), "5.0")

		stopped := make(chan struct{})
		go func() {
			w.Stop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("Stop did not return after a failed Start")
		}
		assert.Equal(t, 1, c.probes)
		assert.Empty(t, c.purgeEvals(), "no worker must run after a failed probe")
		assert.Empty(t, c.evals(), "no worker must run after a failed probe")
	})

	t.Run("transport error is not a version error", func(t *testing.T) {
		c := &wtClient{probeFail: func() error {
			return &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}}
		e := &wtEnv{t: t, opts: wtOptions(), client: c, clock: newWtClock(), log: &wtLogger{}}
		w, err := NewWriter(c, wtKeys(t), wtFingerprint, e.opts, e.log)
		require.NoError(t, err)
		e.w = w

		require.NoError(t, w.Start(nil))
		wtWaitUntil(t, "worker started after a transport error at probe", func() bool { return len(c.purgeEvals()) > 0 })
		e.stop()
		assert.Equal(t, 1, c.probes)
	})
}

// an auth or permission reply at probe keeps its own message and does not claim the server is too old
func TestStartProbeAuthErrorKeepsOwnMessage(t *testing.T) {
	for _, msg := range []string{
		"WRONGPASS invalid username-password pair or user is disabled.",
		"NOPERM this user has no permissions to run the 'eval' command",
	} {
		t.Run(msg, func(t *testing.T) {
			c := &wtClient{probeFail: func() error { return redisReply(msg) }}
			w, err := NewWriter(c, wtKeys(t), wtFingerprint, wtOptions(), &wtLogger{})
			require.NoError(t, err)

			startErr := w.Start(nil)
			var initErr *common.InitializationError
			require.ErrorAs(t, startErr, &initErr)
			assert.Equal(t, common.ExitRedisInitializationFailed, initErr.ExitCode())
			assert.Contains(t, startErr.Error(), msg)
			assert.NotContains(t, startErr.Error(), "5.0")
			w.Stop()
		})
	}
}

// NewWriter rejects keys built for a different prefix than the client uses
func TestNewWriterRejectsPrefixMismatch(t *testing.T) {
	c := &wtClient{}
	keys, err := NewKeys("other", "test-api-key", "")
	require.NoError(t, err)
	_, err = NewWriter(c, keys, wtFingerprint, wtOptions(), &wtLogger{})
	require.ErrorIs(t, err, ErrInvalidOptions)

	keys, err = NewKeys("", "test-api-key", "")
	require.NoError(t, err)
	w, err := NewWriter(c, keys, wtFingerprint, wtOptions(), &wtLogger{})
	require.NoError(t, err)
	w.Stop()
}

// changes that cannot drain by the Stop deadline are dropped and counted in the Info log. While
// they are queued Status reports them. A restart with a Source restores them; without one Redis stays behind.
func TestQueueLossOnStopLogsAndReportsPending(t *testing.T) {
	cases := []struct {
		name string
		src  bool
	}{{"with source", true}, {"nil source", false}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newScriptEnv(t)
			opts := wtOptions()
			opts.BackoffMaxWait = 10 * time.Millisecond
			opts.StopTimeoutMs = 200
			cut := &cutClient{PrefixedRedisClient: e.client}
			log := &wtLogger{}
			w, err := NewWriter(cut, e.keys, wtFingerprint, opts, log)
			require.NoError(t, err)
			t.Cleanup(w.Stop)
			src := newSegDoneSource(&wtSource{flags: wtFlags("f1"), flagsCn: 1})
			if err := w.Start(src); err != nil {
				t.Fatal(err)
			}
			src.wait(t)
			wtWaitUntil(t, "the startup resync", func() bool { return w.Status().PendingChanges == 0 })

			cut.setDown(true)
			w.PutFlags([]dtos.SplitDTO{wtFlag("f2", 2, "ACTIVE")}, nil, 2)
			w.PutSegment("s", []string{"k"}, nil, 1)
			wtWaitUntil(t, "both changes pending behind the failing link", func() bool {
				s := w.Status()
				return s.PendingChanges == 2 && s.LagMs > 0
			})
			w.Stop()
			assert.Contains(t, log.infoText(), "2 queued changes dropped")
			assert.Equal(t, 0, w.Status().PendingChanges, "Stop empties the queue it reported as dropped")

			fk := e.keys.FlagKeys()
			d := e.dump()
			assert.Equal(t, "1", d[e.keys.Meta()].Fields[e.keys.FlagCursorField()], "the dropped flag change never moved the cursor")
			assert.Len(t, d[fk[2]].Scores, 1, "f2 was not written")
			assert.NotContains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("s"), "the dropped segment change has no cursor")

			if tc.src {
				restart, err := NewWriter(&cutClient{PrefixedRedisClient: e.client}, e.keys, wtFingerprint, opts, &wtLogger{})
				require.NoError(t, err)
				t.Cleanup(restart.Stop)
				src2 := newSegDoneSource(&wtSource{flags: wtFlags("f1", "f2"), flagsCn: 2, segs: []SegmentState{
					{Name: "s", Active: map[string]int64{"k": 1}, ChangeNumber: 1},
				}})
				if err := restart.Start(src2); err != nil {
					t.Fatal(err)
				}
				src2.wait(t)
				restart.Stop()

				d = e.dump()
				assert.Equal(t, "2", d[e.keys.Meta()].Fields[e.keys.FlagCursorField()], "the resync restores the dropped flag")
				assert.Len(t, d[fk[2]].Scores, 2)
				assert.Equal(t, "1", d[e.keys.Meta()].Fields[e.keys.SegmentCursorField("s")], "the resync restores the dropped segment")
				return
			}

			restart, err := NewWriter(&cutClient{PrefixedRedisClient: e.client}, e.keys, wtFingerprint, opts, &wtLogger{})
			require.NoError(t, err)
			t.Cleanup(restart.Stop)
			if err := restart.Start(nil); err != nil {
				t.Fatal(err)
			}
			restart.Stop()
			assert.Equal(t, int64(0), restart.Status().Resyncs, "no resync without a Source")

			d = e.dump()
			assert.Equal(t, "1", d[e.keys.Meta()].Fields[e.keys.FlagCursorField()], "without a Source Redis stays behind")
			assert.Len(t, d[fk[2]].Scores, 1)
			assert.NotContains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("s"))
		})
	}
}

// TestPurgeBatchSizesAndRepeatsOnFullBatch: purgeBatch sends Options.PurgeBatch to PurgeScript and purge repeats a target while a call removes a full batch, then moves to the next target.
func TestPurgeBatchSizesAndRepeatsOnFullBatch(t *testing.T) {
	t.Run("full batches repeat the target", func(t *testing.T) {
		opts := wtOptions()
		opts.PurgeBatch = 2
		e := newWtEnv(t, opts)
		e.client.meta = map[string]string{}
		// the first two purge calls remove a full batch of 2, later calls remove nothing
		e.client.purgeFail = func(n int, _ wtCall) error {
			e.client.mu.Lock()
			defer e.client.mu.Unlock()
			if n <= 2 {
				e.client.meta[purgeLastField] = "2"
			}
			return nil
		}
		if err := e.w.Start(&wtSource{flags: wtFlags("f1"), flagsCn: 5}); err != nil {
			t.Fatal(err)
		}
		wtWaitUntil(t, "the purge passes to finish", func() bool { return len(e.client.purgeEvals()) >= 4 })
		e.w.Stop()
		calls := e.client.purgeEvals()
		require.Len(t, calls, 4, "flags: full, full, short (3 calls); rule-based segments: short (1 call)")
		for _, c := range calls {
			assert.Equal(t, int64(2), c.args[2], "batch argument")
		}
	})
}

// restoreGroup puts back the names that releaseGroup cleared after a failed write, merged with any name skipped since then
func TestRestoreGroupMergesNamesAfterFailedWrite(t *testing.T) {
	e, _ := wtResetFixture(t, &wtClient{})
	const field = "flags.till"
	e.w.freezeOf(field).names["a"] = struct{}{}
	old := e.w.releaseGroup(field)
	require.Empty(t, e.w.freezeOf(field).names)
	e.w.freezeOf(field).names["b"] = struct{}{}
	e.w.restoreGroup(field, old)
	g := e.w.freezeOf(field)
	assert.Equal(t, map[string]struct{}{"a": {}, "b": {}}, g.names)
	assert.False(t, g.whole)
}

// restoreGroup keeps a whole-group freeze: the names are dropped and the group stays frozen
func TestRestoreGroupKeepsWholeFreeze(t *testing.T) {
	e, _ := wtResetFixture(t, &wtClient{})
	const field = "flags.till"
	g := e.w.freezeOf(field)
	g.names["a"] = struct{}{}
	g.whole = true
	old := e.w.releaseGroup(field)
	require.False(t, e.w.freezeOf(field).whole)
	e.w.freezeOf(field).names["c"] = struct{}{}
	e.w.restoreGroup(field, old)
	g = e.w.freezeOf(field)
	assert.True(t, g.whole)
	assert.Empty(t, g.names)
}

// a meta fingerprint that differs from the writer's stops the startup purge before any PurgeScript call
func TestStartupPurgeFingerprintMismatchSkipsPurge(t *testing.T) {
	c := &wtClient{}
	e, _ := wtResetFixture(t, c)
	c.meta = map[string]string{FingerprintField: "other-fp"}
	err := e.w.startupPurge()
	assert.ErrorIs(t, err, ErrEnvironmentMismatch)
	assert.Empty(t, c.purgeEvals())
}

// a purge that fails is logged and returns nil, so the worker goes on and the purge reruns at the next start
func TestStartupPurgeLogsFailureAndReturnsNil(t *testing.T) {
	c := &wtClient{}
	c.purgeFail = func(int, wtCall) error { return errors.New("boom") }
	e, _ := wtResetFixture(t, c)
	c.meta = map[string]string{}
	assert.NoError(t, e.w.startupPurge())
	assert.GreaterOrEqual(t, e.log.errorCount(), 1)
}

// a full PurgeBatch removal repeats the PurgeScript call until a short batch is removed
func TestStartupPurgeRepeatsWhileBatchIsFull(t *testing.T) {
	c := &wtClient{}
	c.purgeFail = func(n int, _ wtCall) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if n == 1 {
			c.meta[purgeLastField] = "2"
		}
		return nil
	}
	e, _ := wtResetFixture(t, c)
	c.meta = map[string]string{}
	e.w.opts.PurgeBatch = 2
	require.NoError(t, e.w.startupPurge())
	calls := c.purgeEvals()
	require.Len(t, calls, 3)
	for _, call := range calls {
		assert.Equal(t, int64(2), call.args[2])
	}
}
