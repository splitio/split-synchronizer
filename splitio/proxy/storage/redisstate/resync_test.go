package redisstate

import (
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResyncWritesFullStateFromSource(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	src := &wtSource{
		flags:   []dtos.SplitDTO{wtFlag("on", 10, "ACTIVE"), wtFlag("old", 11, "ARCHIVED")},
		flagsCn: 11,
		rbs:     []dtos.RuleBasedSegmentDTO{{Name: "rb", ChangeNumber: 12, Status: "ACTIVE"}},
		rbsCn:   12,
		segs: []SegmentState{{
			Name:         "s1",
			Active:       map[string]int64{"k1": 3, "k2": 4},
			Removed:      map[string]int64{"k3": 5},
			ChangeNumber: 6,
		}},
	}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, []string{e.w.keys.MetaUnprefixed()}, e.client.hgetKeys(), "meta is read once per resync")
	assert.Equal(t, int64(0), e.w.Status().Resyncs, "the startup resync is not counted")

	calls := e.client.evals()
	require.Len(t, calls, 3)

	flagCall := calls[0]
	assert.Equal(t, FlagScript, flagCall.script)
	assert.Equal(t, int64(11), wtFlagCursor(flagCall.args))
	flagEnts := map[string]wtEnt{}
	for _, en := range wtEntities(flagCall.args) {
		flagEnts[en.name] = en
	}
	require.Len(t, flagEnts, 2)
	assert.Equal(t, "0", flagEnts["on"].removed)
	assert.Equal(t, "1", flagEnts["old"].removed, "ARCHIVED is removed")

	rbsCall := calls[1]
	assert.Equal(t, RuleBasedSegmentScript, rbsCall.script)
	assert.Equal(t, int64(12), wtFlagCursor(rbsCall.args))

	segCall := calls[2]
	assert.Equal(t, SegmentScript, segCall.script)
	name, cursor, adds, rems := wtSegArgs(segCall.args)
	assert.Equal(t, "s1", name)
	assert.Equal(t, int64(6), cursor)
	assert.Equal(t, []wtSegKey{{key: "k1", cn: 3}, {key: "k2", cn: 4}}, adds, "per-key change numbers")
	assert.Equal(t, []wtSegKey{{key: "k3", cn: 5}}, rems)
}

func TestResyncSourceErrorIsRetried(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	src := &wtSource{
		flags:      wtFlags("f1"),
		flagsCn:    1,
		flagsFails: 2,
		segs:       []SegmentState{{Name: "s", Active: map[string]int64{"k": 1}, ChangeNumber: 1}},
		segsFails:  1,
	}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Equal(t, 3, src.flagsCalls, "two failed reads, then a successful one")
	assert.Equal(t, int64(0), e.w.Status().FailedRounds, "source errors are retried inside the round")
	assert.Equal(t, 3, e.log.errorCount(), "two flag read errors and one segment listing error are logged")
	calls := e.client.evals()
	require.Len(t, calls, 3)
	assert.Equal(t, SegmentScript, calls[2].script)
	name, _, _, _ := wtSegArgs(calls[2].args)
	assert.Equal(t, "s", name, "the segment is written after the retried listing")
}

func TestResyncSkipsUpToDateSegments(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	meta := map[string]string{}
	meta[e.w.keys.SegmentCursorField("up")] = "6"
	meta[e.w.keys.SegmentCursorField("ahead")] = "9"
	meta[e.w.keys.SegmentCursorField("behind")] = "5"
	meta[e.w.keys.SegmentCursorField("bogus")] = "abc"
	e.client.meta = meta
	seg := func(name string) SegmentState {
		return SegmentState{Name: name, Active: map[string]int64{"k": 6}, ChangeNumber: 6}
	}
	src := &wtSource{segs: []SegmentState{seg("up"), seg("ahead"), seg("behind"), seg("bogus"), seg("absent")}}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 5 && e.w.Status().PendingChanges == 0 })
	e.stop()

	written := map[string]bool{}
	for _, c := range e.client.evals() {
		if c.script == SegmentScript {
			name, _, _, _ := wtSegArgs(c.args)
			written[name] = true
		}
	}
	assert.Equal(t, map[string]bool{"behind": true, "bogus": true, "absent": true}, written)
	assert.Equal(t, []string{e.w.keys.MetaUnprefixed()}, e.client.hgetKeys())
}

func TestResyncDetectsMismatchBeforeWriting(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.meta = map[string]string{"fingerprint": "another-configuration"}
	src := &wtSource{flags: wtFlags("f1"), flagsCn: 1, segs: []SegmentState{{Name: "s", ChangeNumber: 1}}}
	e.w.Start(src)
	wtWaitUntil(t, "the mismatch", func() bool { return e.w.Status().EnvironmentMismatch })
	e.stop()

	assert.Equal(t, 0, e.client.count(), "nothing is written when the fingerprint differs")
	assert.Equal(t, 0, src.flagsCalls, "the source is not read")
	assert.GreaterOrEqual(t, e.log.errorCount(), 1)
}

func TestResyncSkipsBadArgsChunkWithoutRetry(t *testing.T) {
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
	src := &wtSource{flags: wtFlags("a", "b", "c"), flagsCn: 5}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 4 })
	e.stop()

	bCalls := 0
	for _, c := range e.client.evals() {
		if ents := wtEntities(c.args); len(ents) == 1 && ents[0].name == "b" {
			bCalls++
		}
	}
	assert.Equal(t, 1, bCalls, "a refused chunk is not retried")
	assert.Equal(t, 1, src.flagsCalls, "a refused chunk during a resync does not schedule another pass")
	assert.Equal(t, int64(1), e.w.Status().RejectedChanges)
}

func TestResyncIsolatesBadEntity(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.fail = func(_ int, c wtCall) error {
		if wtHasArg(c.args, "bad") {
			return errors.New("PROXY_BAD_ARGS")
		}
		return nil
	}
	src := &wtSource{
		flags:   wtFlags("a", "bad", "c"),
		flagsCn: 5,
		segs:    []SegmentState{{Name: "s", Active: map[string]int64{"bad": 3, "g1": 3}, ChangeNumber: 3}},
	}
	e.w.Start(src)
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 8 })
	e.stop()

	var flagsWritten, keysWritten []string
	for _, c := range e.client.evals() {
		switch c.script {
		case FlagScript:
			if c.args[2] != e.w.keys.FlagCursorField() || wtHasArg(c.args, "bad") {
				continue
			}
			assert.Equal(t, int64(-1), wtFlagCursor(c.args), "no accepted call carries the cursor past a skipped entity")
			for _, en := range wtEntities(c.args) {
				flagsWritten = append(flagsWritten, en.name)
			}
		case SegmentScript:
			_, cursor, adds, _ := wtSegArgs(c.args)
			if wtHasArg(c.args, "bad") {
				continue
			}
			assert.Equal(t, int64(-1), cursor, "no accepted call carries the cursor past a skipped key")
			for _, k := range adds {
				keysWritten = append(keysWritten, k.key)
			}
		}
	}
	assert.Equal(t, []string{"a", "c"}, flagsWritten, "the good flags are written")
	assert.Equal(t, []string{"g1"}, keysWritten, "the good segment key is written")
	assert.Equal(t, int64(2), e.w.Status().RejectedChanges, "one refused flag and one refused key")
	assert.Equal(t, 1, src.flagsCalls, "a refusal during a resync does not schedule another pass")
}

func TestResyncRewritesSegmentWithCurrentCursorButMissingKeys(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.meta = map[string]string{e.w.keys.SegmentCursorField("up"): "6"}
	e.client.checkFail = func() error { return errors.New("ERR PROXY_COUNT_MISMATCH") }
	src := &wtSource{segs: []SegmentState{{Name: "up", Active: map[string]int64{"k": 6}, ChangeNumber: 6}}}
	e.w.Start(src)
	wtWaitUntil(t, "the rewrite", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	assert.Len(t, e.client.checks, 1)
	assert.Equal(t, SegmentScript, e.client.evals()[2].script)
}

// segDoneSource closes done once the segment listing has finished, which is after the last segment was written
type segDoneSource struct {
	*wtSource
	once   sync.Once
	done   chan struct{}
	passes atomic.Int32
}

func newSegDoneSource(src *wtSource) *segDoneSource {
	return &segDoneSource{wtSource: src, done: make(chan struct{})}
}

func (s *segDoneSource) Segments(visit func(SegmentState) error) error {
	err := s.wtSource.Segments(visit)
	if err == nil {
		s.passes.Add(1)
		s.once.Do(func() { close(s.done) })
	}
	return err
}

// newRealWriter builds a Writer that talks to the Redis of the script env
func newRealWriter(t *testing.T, e *scriptEnv, opts Options) (*Writer, *wtLogger) {
	t.Helper()
	log := &wtLogger{}
	w, err := NewWriter(e.client, e.keys, wtFingerprint, opts, log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return w, log
}

func (s *segDoneSource) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the segment resync")
	}
}

// gatedClient is a real client whose next Eval can be held until the test releases it
type gatedClient struct {
	*redis.PrefixedRedisClient
	mu      sync.Mutex
	gate    chan struct{}
	blocked chan struct{}
}

// holdNext makes the next Eval block; blocked is closed once it does
func (c *gatedClient) holdNext() (release func(), blocked <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gate = make(chan struct{})
	c.blocked = make(chan struct{})
	var once sync.Once
	gate := c.gate
	return func() { once.Do(func() { close(gate) }) }, c.blocked
}

func (c *gatedClient) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	gate, blocked := c.gate, c.blocked
	if script != PurgeScript {
		c.gate, c.blocked = nil, nil
	} else {
		gate = nil
	}
	c.mu.Unlock()
	if gate != nil {
		close(blocked)
		<-gate
	}
	return c.PrefixedRedisClient.Eval(script, keys, args...)
}

func TestOverflowThenLiveChangeThenResyncConverges(t *testing.T) {
	e := newScriptEnv(t)
	opts := wtOptions()
	opts.MaxPendingChanges = 1
	client := &gatedClient{PrefixedRedisClient: e.client}
	log := &wtLogger{}
	w, err := NewWriter(client, e.keys, wtFingerprint, opts, log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)

	state := &wtSource{
		flags:   []dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE")},
		flagsCn: 1,
		segs:    []SegmentState{{Name: "s", Active: map[string]int64{"k1": 1}, ChangeNumber: 1}},
	}
	src := newSegDoneSource(state)
	w.Start(src)
	src.wait(t)
	wtWaitUntil(t, "the startup resync", func() bool { return w.Status().PendingChanges == 0 })

	release, blocked := client.holdNext()
	t.Cleanup(release)
	w.PutFlags([]dtos.SplitDTO{wtFlag("f2", 2, "ACTIVE")}, nil, 2)
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not pick up the flag change")
	}

	w.PutSegment("s", []string{"k2"}, nil, 2)
	w.PutSegment("s", []string{"k3"}, nil, 3)
	require.Equal(t, int64(1), w.Status().Resyncs, "the second segment change overflows the queue")
	require.Equal(t, 2, w.Status().PendingChanges, "the queue dropped k2; only the in-flight flag change and the kept k3 remain")
	require.Equal(t, []string{"k3"}, queuedSegmentAdds(w), "the overflow emptied the queue, so k2 is gone and only k3 is queued")
	w.PutSegment("s", []string{"k4"}, nil, 4)
	require.Equal(t, int64(2), w.Status().Resyncs, "k4 overflows again and drops the queued k3")
	require.Equal(t, 2, w.Status().PendingChanges, "the queue dropped k3; only the in-flight flag change and k4 remain")
	require.Equal(t, []string{"k4"}, queuedSegmentAdds(w), "the second overflow emptied the queue, so only k4 is queued")

	// The resync reads the source before the queued k4 is written, so k4 reaches Redis only through the live change.
	state.flags = []dtos.SplitDTO{wtFlag("f1", 1, "ACTIVE"), wtFlag("f2", 2, "ACTIVE")}
	state.flagsCn = 2
	state.segs = []SegmentState{{Name: "s", Active: map[string]int64{"k1": 1, "k2": 2, "k3": 3}, ChangeNumber: 3}}
	release()
	wtWaitUntil(t, "the second resync and the live change", func() bool {
		return src.passes.Load() >= 2 && w.Status().PendingChanges == 0
	})
	w.Stop()

	d := e.dump()
	sk := e.keys.SegmentKeys("s")
	fk := e.keys.FlagKeys()
	assert.Equal(t, map[string]float64{"f1": 1, "f2": 2}, d[fk[2]].Scores)
	assert.Equal(t, map[string]float64{"k1": 1, "k2": 2, "k3": 3, "k4": 4}, d[sk[1]].Scores, "the dropped key change is restored")
	assert.Equal(t, "2", d[fk[0]].Fields[e.keys.FlagCursorField()])
	assert.Equal(t, "4", d[sk[0]].Fields[e.keys.SegmentCursorField("s")])
	for field := range d[fk[0]].Fields {
		assert.NotContains(t, field, "dirty.", "nothing was skipped, so no marker")
	}
	assert.NotContains(t, d, sk[2], "no tombstones")
}

// queuedSegmentAdds returns the keys waiting to be added in the queued segment changes. The caller must hold the
// worker blocked so the queue is stable.
func queuedSegmentAdds(w *Writer) []string {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	var keys []string
	for _, c := range w.queue {
		for _, k := range c.adds {
			keys = append(keys, k.key)
		}
	}
	return keys
}

// T59: a requested resync with a populated source rewrites the groups Redis lacks, with no marker field
func TestRequestResyncRewritesFlagsFromPopulatedSource(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(wtFingerprint, 100, ent{"a", 100, false, `{"name":"a"}`}))
	require.Nil(t, e.rbs(wtFingerprint, 100, ent{"r", 100, false, `{"name":"r"}`}))
	w, _ := newRealWriter(t, e, wtOptions())
	state := &wtSource{flagsCn: -1, rbsCn: -1}
	src := newSegDoneSource(state)
	w.Start(src)
	src.wait(t)

	state.flags = []dtos.SplitDTO{wtFlag("a", 100, "ACTIVE"), wtFlag("b", 100, "ACTIVE")}
	state.flagsCn = 100
	state.rbsCn = 100
	w.RequestResync()
	wtWaitUntil(t, "the requested resync", func() bool { return src.passes.Load() >= 2 })
	w.Stop()

	d := e.dump()
	meta := d[e.keys.Meta()].Fields
	assert.Equal(t, "100", meta[e.keys.FlagCursorField()])
	assert.Equal(t, "100", meta[e.keys.RuleBasedSegmentCursorField()])
	for f := range meta {
		assert.False(t, strings.HasPrefix(f, "dirty."), "no marker field, found %s", f)
	}
	assert.Equal(t, float64(100), d[e.keys.FlagKeys()[2]].Scores["b"], "the flag missing from Redis is rewritten")
}

// T62: a requested resync against an empty or behind source changes nothing: no key is added and no cursor moves back
func TestRequestResyncEmptySourceChangesNothing(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(wtFingerprint, 100, ent{"a", 100, false, `{"name":"a"}`}))
	require.Nil(t, e.rbs(wtFingerprint, 100, ent{"r", 100, false, `{"name":"r"}`}))
	w, _ := newRealWriter(t, e, wtOptions())
	state := &wtSource{flagsCn: -1, rbsCn: -1}
	src := newSegDoneSource(state)
	w.Start(src)
	src.wait(t)

	w.RequestResync()
	wtWaitUntil(t, "the resync with the empty source", func() bool { return src.passes.Load() >= 2 })
	state.flags = []dtos.SplitDTO{wtFlag("a", 50, "ACTIVE")}
	state.flagsCn = 50
	w.RequestResync()
	wtWaitUntil(t, "the resync with the behind source", func() bool { return src.passes.Load() >= 3 })
	w.Stop()

	d := e.dump()
	meta := d[e.keys.Meta()].Fields
	assert.Equal(t, "100", meta[e.keys.FlagCursorField()], "the cursor never moves back")
	assert.Equal(t, "100", meta[e.keys.RuleBasedSegmentCursorField()])
	assert.Equal(t, float64(100), d[e.keys.FlagKeys()[2]].Scores["a"], "the behind source does not lower a flag")
	assert.Len(t, d[e.keys.FlagKeys()[2]].Scores, 1, "the behind source adds no flag")
}

// T65: a flags and segment write followed by a resync leaves no dirty.* field and no key with "dirty" in the namespace
func TestNoDirtyFieldsWritten(t *testing.T) {
	e := newScriptEnv(t)
	w, _ := newRealWriter(t, e, wtOptions())
	state := &wtSource{
		flags:   []dtos.SplitDTO{wtFlag("on", 10, "ACTIVE")},
		flagsCn: 10,
		rbsCn:   -1,
		segs:    []SegmentState{{Name: "s1", Active: map[string]int64{"k1": 3}, Removed: map[string]int64{"k2": 4}, ChangeNumber: 5}},
	}
	src := newSegDoneSource(state)
	w.Start(src)
	src.wait(t)
	state.flagsCn = 11
	w.RequestResync()
	wtWaitUntil(t, "the requested resync", func() bool { return src.passes.Load() >= 2 && w.Status().PendingChanges == 0 })
	w.Stop()

	d := e.dump()
	require.Contains(t, d, e.keys.Meta(), "the writes reached Redis")
	for f := range d[e.keys.Meta()].Fields {
		assert.False(t, strings.HasPrefix(f, "dirty."), "no dirty.* field in meta, found %s", f)
	}
	for k := range d {
		assert.NotContains(t, strings.TrimPrefix(k, e.prefix), "dirty", "no dirty key in the namespace, found %s", k)
	}
}

// recordingClient is a real client that records every Eval (script and arguments) in call order
type recordingClient struct {
	*redis.PrefixedRedisClient
	mu    sync.Mutex
	evals []recordedEval
}

type recordedEval struct {
	script string
	args   []interface{}
}

func (c *recordingClient) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	c.evals = append(c.evals, recordedEval{script: script, args: args})
	c.mu.Unlock()
	return c.PrefixedRedisClient.Eval(script, keys, args...)
}

func (c *recordingClient) recorded() []recordedEval {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedEval(nil), c.evals...)
}

// dormantRemovedGone reports whether no tombstone of the segment named dormant still holds the member gone
func dormantRemovedGone(e *scriptEnv) bool {
	d := e.dump()
	for _, k := range e.keys.SegmentKeys("dormant") {
		if v, ok := d[k]; ok {
			if _, has := v.Scores["gone"]; has {
				return false
			}
		}
	}
	return true
}

func TestStartPurgesDormantTombstones(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.segment(testFp, "dormant", 3, nil, []segKey{{"gone", 4}}))
	time.Sleep(5 * time.Millisecond)

	opts := wtOptions()
	opts.Retention = time.Millisecond
	rec := &recordingClient{PrefixedRedisClient: e.client}
	w, err := NewWriter(rec, e.keys, testFp, opts, &wtLogger{})
	require.NoError(t, err)
	src := &wtSource{flags: []dtos.SplitDTO{wtFlag("on", 10, "ACTIVE")}, flagsCn: 10}
	w.Start(src)
	wtWaitUntil(t, "the startup purge of the dormant tombstone", func() bool { return dormantRemovedGone(e) && w.Status().PendingChanges == 0 })
	w.Stop()

	scripts := []string{}
	for _, c := range rec.recorded() {
		scripts = append(scripts, c.script)
	}
	resyncAt, purgeAt := slices.Index(scripts, FlagScript), slices.Index(scripts, PurgeScript)
	require.GreaterOrEqual(t, resyncAt, 0, "the first resync writes the flags")
	require.GreaterOrEqual(t, purgeAt, 0, "the startup purge runs")
	assert.Greater(t, purgeAt, resyncAt, "D27 REV 19c: the purge runs after the first resync completes")
}

func TestPurgeBatchConfigurable(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(testFp, 5, ent{"g1", 6, true, `{}`}, ent{"g2", 7, true, `{}`}, ent{"g3", 8, true, `{}`}))
	time.Sleep(5 * time.Millisecond)

	opts := wtOptions()
	opts.Retention = time.Millisecond
	opts.PurgeBatch = 1
	rec := &recordingClient{PrefixedRedisClient: e.client}
	w, err := NewWriter(rec, e.keys, testFp, opts, &wtLogger{})
	require.NoError(t, err)
	require.NoError(t, w.purge(map[string]string{}))

	fk := e.keys.FlagKeys()
	d := e.dump()
	for _, name := range []string{"g1", "g2", "g3"} {
		assert.NotContains(t, d[fk[1]].Fields, name, "a batch of 1 still purges every expired removal")
	}
	purges := 0
	for _, c := range rec.recorded() {
		if c.script != PurgeScript {
			continue
		}
		purges++
		assert.Equal(t, int64(1), c.args[2], "the batch value reaches PurgeScript as ARGV[3]")
	}
	assert.Equal(t, 5, purges, "flags: three calls removing one entry each, then one empty call; rule-based segments: one empty call")
}

// T149 (D33): a segment group first seen after an overflow starts frozen. Its first live write sends noCursor, and it
// stays frozen until a resync with a Source releases it. The overflow resync fails permanently (flags read on call 2), so
// the overflow is not cleared before the live write.
func TestOverflowFreezesGroupFirstSeenAfterOverflow(t *testing.T) {
	opts := wtOptions()
	opts.MaxPendingChanges = 1
	e := newWtEnv(t, opts)
	gate := make(chan struct{})
	blocked := make(chan struct{})
	var blockOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	e.client.fail = func(_ int, c wtCall) error {
		if c.script == SegmentScript && wtHasArg(c.args, "k2") {
			blockOnce.Do(func() { close(blocked) })
			<-gate
		}
		return nil
	}
	ws := &wtSource{
		flags:   wtFlags("f1"),
		flagsCn: 1,
		segs:    []SegmentState{{Name: "s", Active: map[string]int64{"k1": 1}, ChangeNumber: 1}},
		permAt:  2,
		permErr: errors.New("WRONGTYPE source flags unreadable"),
	}
	src := newSegDoneSource(ws)
	e.w.Start(src)
	src.wait(t)
	wtWaitUntil(t, "the startup resync", func() bool { return e.w.Status().PendingChanges == 0 })
	base := e.client.count()

	e.w.PutSegment("s", []string{"k2"}, nil, 2)
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not pick up the first segment change")
	}
	e.w.PutSegment("s", []string{"k3"}, nil, 3)
	e.w.PutSegment("n", []string{"k1"}, nil, 4)
	require.Equal(t, int64(1), e.w.Status().Resyncs, "the change of group n overflows the queue")
	release()
	wtWaitUntil(t, "the live write of group n", func() bool { return e.client.count() == base+2 && e.w.Status().PendingChanges == 0 })

	last := e.client.evals()[base+1]
	name, cursor, _, _ := wtSegArgs(last.args)
	require.Equal(t, "n", name)
	assert.Equal(t, noCursor, cursor, "the first write of a group seen after the overflow sends noCursor")

	ws.segs = append(ws.segs, SegmentState{Name: "n", Active: map[string]int64{"k1": 1}, ChangeNumber: 4})
	e.w.RequestResync()
	e.w.PutSegment("n", []string{"k2"}, nil, 5)
	wtWaitUntil(t, "the write after the resync", func() bool {
		calls := e.client.evals()
		return e.w.Status().PendingChanges == 0 && len(calls) > base+2 && calls[len(calls)-1].script == SegmentScript
	})
	calls := e.client.evals()
	name, cursor, _, _ = wtSegArgs(calls[len(calls)-1].args)
	assert.Equal(t, "n", name)
	assert.Equal(t, int64(5), cursor, "a resync with a Source releases group n, so the next write moves its cursor")
}

// T148 (D34, D19): after a skipped entity and a process restart, a resync with a Source rewrites the group and releases
// the cursor. With a nil Source nothing repairs the group (limit A3), so the skipped entity is never sent again.
func TestRestartAfterSkippedEntityRepairsGroupOnlyWithSource(t *testing.T) {
	cases := []struct {
		name string
		src  bool
	}{{"with source", true}, {"nil source", false}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newWtEnv(t, wtOptions())
			refuse := true
			e.client.fail = func(_ int, c wtCall) error {
				if refuse && wtHasArg(c.args, "bad") {
					return errors.New("PROXY_BAD_ARGS")
				}
				return nil
			}
			e.w.Start(&wtSource{flags: wtFlags("a", "bad", "c"), flagsCn: 5})
			wtWaitUntil(t, "the first resync", func() bool { return e.w.Status().RejectedChanges == 1 && e.w.Status().PendingChanges == 0 })
			e.stop()

			refuse = false
			mark := len(e.client.evals())
			w2, err := NewWriter(e.client, e.w.keys, wtFingerprint, wtOptions(), &wtLogger{})
			require.NoError(t, err)
			t.Cleanup(w2.Stop)
			var src Source
			if tc.src {
				src = &wtSource{flags: wtFlags("a", "bad", "c"), flagsCn: 5}
			}
			w2.Start(src)

			if !tc.src {
				w2.Stop()
				assert.Equal(t, int64(0), w2.Status().Resyncs)
				for _, c := range e.client.evals()[mark:] {
					assert.False(t, c.script == FlagScript && wtHasArg(c.args, "bad"), "nil source: the skipped entity is never sent again")
				}
				return
			}

			wtWaitUntil(t, "the restart resync", func() bool {
				for _, c := range e.client.evals()[mark:] {
					if c.script == FlagScript && c.args[2] == e.w.keys.FlagCursorField() && wtFlagCursor(c.args) == 5 {
						return true
					}
				}
				return false
			})
			e.w.Stop()
			w2.Stop()
			var repaired bool
			for _, c := range e.client.evals()[mark:] {
				if c.script == FlagScript && c.args[2] == e.w.keys.FlagCursorField() && wtFlagCursor(c.args) == 5 {
					repaired = wtHasArg(c.args, "bad")
				}
			}
			assert.True(t, repaired, "the resync with a Source rewrites the skipped entity under the released cursor")
		})
	}
}

// T148 same-writer case (F4): the group freeze lives in memory, so only the instance that skipped the entity can
// release it. A RequestResync on that instance with a populated Source must rewrite the skipped entity under the
// released cursor (releaseGroup, writer.go).
func TestRequestResyncRepairsSkippedEntityOnSameWriter(t *testing.T) {
	cases := []struct {
		name     string
		after    []string // the flags the Source holds when the repair resync runs
		repaired bool
	}{
		{"source still holds the skipped entity", []string{"a", "bad", "c"}, true},
		{"source drops the skipped entity", []string{"a", "c"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newWtEnv(t, wtOptions())
			refuse := true
			e.client.fail = func(_ int, c wtCall) error {
				if refuse && wtHasArg(c.args, "bad") {
					return errors.New("PROXY_BAD_ARGS")
				}
				return nil
			}
			src := &wtSource{flags: wtFlags("a", "bad", "c"), flagsCn: 5}
			e.w.Start(src)
			wtWaitUntil(t, "the first resync", func() bool { return e.w.Status().RejectedChanges == 1 && e.w.Status().PendingChanges == 0 })

			refuse = false
			src.flags = wtFlags(tc.after...)
			mark := len(e.client.evals())
			e.w.RequestResync()
			released := func() bool {
				for _, c := range e.client.evals()[mark:] {
					if c.script == FlagScript && c.args[2] == e.w.keys.FlagCursorField() && wtFlagCursor(c.args) == 5 {
						return true
					}
				}
				return false
			}
			wtWaitUntil(t, "the repair resync under cursor 5", released)
			var sentBad bool
			for _, c := range e.client.evals()[mark:] {
				if c.script == FlagScript && wtHasArg(c.args, "bad") {
					sentBad = true
				}
			}
			assert.Equal(t, tc.repaired, sentBad, "the repair resync rewrites the skipped entity only when the source still holds it")
		})
	}
}

// T151 (D34 A1, pinning test): when the meta hash disappears mid-run (Redis data loss, residual A1), the next write
// recreates the fingerprint and writes the cursor. Detection is out of scope; a later change that makes this write refuse
// must be a deliberate decision, so this test fails on it.
func TestFingerprintMissingMidRunRecreatedByNextWrite(t *testing.T) {
	e := newScriptEnv(t)
	w, _ := newRealWriter(t, e, wtOptions())
	src := newSegDoneSource(&wtSource{flags: wtFlags("f1"), flagsCn: 1})
	w.Start(src)
	src.wait(t)
	wtWaitUntil(t, "the startup resync", func() bool { return w.Status().PendingChanges == 0 })

	_, err := e.client.Del(e.keys.MetaUnprefixed())
	require.NoError(t, err)
	w.PutFlags([]dtos.SplitDTO{wtFlag("f2", 2, "ACTIVE")}, nil, 2)
	wtWaitUntil(t, "the write after the fingerprint loss", func() bool { return w.Status().PendingChanges == 0 })
	w.Stop()

	d := e.dump()
	fk := e.keys.FlagKeys()
	assert.Equal(t, wtFingerprint, d[e.keys.Meta()].Fields[FingerprintField], "the next write recreates the fingerprint")
	assert.Equal(t, "2", d[fk[0]].Fields[e.keys.FlagCursorField()], "the next write writes the cursor")
}

// cutClient is a real client whose link can be cut: while down, every Eval is refused. cutAfter cuts the link once
// that many segment writes went through (0 never cuts). It records each Eval so a test can see what was sent.
type cutClient struct {
	*redis.PrefixedRedisClient
	mu       sync.Mutex
	down     bool
	cutAfter int
	segs     int
	refused  bool
	evals    []cutEval
}

type cutEval struct {
	script string
	keys   []string
}

func (c *cutClient) Eval(script string, keys []string, args ...interface{}) error {
	c.mu.Lock()
	c.evals = append(c.evals, cutEval{script: script, keys: keys})
	if script == SegmentScript {
		c.segs++
		if c.cutAfter > 0 && c.segs > c.cutAfter {
			c.down = true
		}
	}
	if c.down {
		c.refused = true
		c.mu.Unlock()
		return &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}
	c.mu.Unlock()
	return c.PrefixedRedisClient.Eval(script, keys, args...)
}

func (c *cutClient) setDown(down bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.down = down
}

func (c *cutClient) wasRefused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refused
}

func (c *cutClient) recorded() []cutEval {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cutEval(nil), c.evals...)
}

// T146 (CP-5, D34/D19): a resync cut off mid pass leaves no cursor ahead of its data, and the restart skips only the
// segments whose cursor is current and whose key count matches.
func TestResyncCrashMidwayRestartKeepsCursorsBehindData(t *testing.T) {
	e := newScriptEnv(t)
	opts := wtOptions()
	opts.BackoffMaxWait = 10 * time.Millisecond
	opts.StopTimeoutMs = 200
	newSource := func() *wtSource {
		return &wtSource{flags: wtFlags("f1", "f2"), flagsCn: 2, segs: []SegmentState{
			{Name: "s1", Active: map[string]int64{"a": 1}, ChangeNumber: 1},
			{Name: "s2", Active: map[string]int64{"b": 2, "c": 2}, ChangeNumber: 2},
			{Name: "s3", Active: map[string]int64{"d": 3}, ChangeNumber: 3},
		}}
	}

	cut := &cutClient{PrefixedRedisClient: e.client, cutAfter: 1}
	w, err := NewWriter(cut, e.keys, wtFingerprint, opts, &wtLogger{})
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	w.Start(newSource())
	wtWaitUntil(t, "the link cut inside the segment pass", cut.wasRefused)
	w.Stop()

	d := e.dump()
	meta := d[e.keys.Meta()].Fields
	fk := e.keys.FlagKeys()
	assert.Equal(t, "2", meta[e.keys.FlagCursorField()], "the flags cursor is written together with the flags")
	assert.Len(t, d[fk[2]].Scores, 2, "both flags are in Redis")
	assert.Equal(t, "1", meta[e.keys.SegmentCursorField("s1")], "s1 was written before the cut")
	assert.Len(t, d[e.keys.SegmentKeys("s1")[1]].Scores, 1)
	assert.NotContains(t, meta, e.keys.SegmentCursorField("s2"), "the cut segment has no cursor")
	assert.NotContains(t, meta, e.keys.SegmentCursorField("s3"), "a segment after the cut has no cursor")
	assert.Empty(t, d[e.keys.SegmentKeys("s2")[1]].Scores, "no data for the cut segment")
	assert.Empty(t, d[e.keys.SegmentKeys("s3")[1]].Scores, "no data for a later segment")

	restart := &cutClient{PrefixedRedisClient: e.client}
	w2, err := NewWriter(restart, e.keys, wtFingerprint, opts, &wtLogger{})
	require.NoError(t, err)
	t.Cleanup(w2.Stop)
	src2 := newSegDoneSource(newSource())
	w2.Start(src2)
	src2.wait(t)
	w2.Stop()

	var checked, written []string
	for _, c := range restart.recorded() {
		switch c.script {
		case SegmentCheckScript:
			checked = append(checked, c.keys[1])
		case SegmentScript:
			written = append(written, c.keys[1])
		}
	}
	assert.Equal(t, []string{e.keys.SegmentKeys("s1")[1]}, checked, "only the segment with a current cursor is checked")
	assert.ElementsMatch(t, []string{e.keys.SegmentKeys("s2")[1], e.keys.SegmentKeys("s3")[1]}, written, "the restart skips s1 and rewrites s2 and s3")

	d = e.dump()
	meta = d[e.keys.Meta()].Fields
	assert.Equal(t, "1", meta[e.keys.SegmentCursorField("s1")])
	assert.Equal(t, "2", meta[e.keys.SegmentCursorField("s2")])
	assert.Equal(t, "3", meta[e.keys.SegmentCursorField("s3")])
	assert.Len(t, d[e.keys.SegmentKeys("s2")[1]].Scores, 2)
	assert.Len(t, d[e.keys.SegmentKeys("s3")[1]].Scores, 1)
}
