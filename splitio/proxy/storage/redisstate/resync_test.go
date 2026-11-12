package redisstate

import (
	"errors"
	"fmt"
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the resync", func() bool { return e.client.count() == 3 && e.w.Status().PendingChanges == 0 })
	e.stop()

	// a resync reads meta, then the flag and rule-based segment data hashes
	assert.Equal(t, []string{e.w.keys.MetaUnprefixed(), e.w.keys.Unprefixed().FlagData(), e.w.keys.Unprefixed().RuleBasedSegmentData()}, e.client.hgetKeys(), "meta is read once per resync, then the flag and rule-based segment hashes")
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	// the resync reads meta, then the flag and rule-based segment hashes, before it decides which segments to skip
	assert.Equal(t, []string{e.w.keys.MetaUnprefixed(), e.w.keys.Unprefixed().FlagData(), e.w.keys.Unprefixed().RuleBasedSegmentData()}, e.client.hgetKeys())
}

func TestResyncDetectsMismatchBeforeWriting(t *testing.T) {
	e := newWtEnv(t, wtOptions())
	e.client.meta = map[string]string{"fingerprint": "another-configuration"}
	src := &wtSource{flags: wtFlags("f1"), flagsCn: 1, segs: []SegmentState{{Name: "s", ChangeNumber: 1}}}
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	calls  atomic.Int32
}

func newSegDoneSource(src *wtSource) *segDoneSource {
	return &segDoneSource{wtSource: src, done: make(chan struct{})}
}

func (s *segDoneSource) Segments(visit func(SegmentState) error) error {
	s.calls.Add(1)
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
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
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

// a requested resync with a populated source rewrites the groups Redis lacks, with no marker field
func TestRequestResyncRewritesFlagsFromPopulatedSource(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(wtFingerprint, 100, ent{"a", 100, false, `{"name":"a"}`}))
	require.Nil(t, e.rbs(wtFingerprint, 100, ent{"r", 100, false, `{"name":"r"}`}))
	w, _ := newRealWriter(t, e, wtOptions())
	state := &wtSource{flagsCn: -1, rbsCn: -1}
	src := newSegDoneSource(state)
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
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

// a requested resync against an empty or behind source changes nothing: no key is added and no cursor moves back
func TestRequestResyncEmptySourceChangesNothing(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(wtFingerprint, 100, ent{"a", 100, false, `{"name":"a"}`}))
	require.Nil(t, e.rbs(wtFingerprint, 100, ent{"r", 100, false, `{"name":"r"}`}))
	w, _ := newRealWriter(t, e, wtOptions())
	state := &wtSource{flagsCn: -1, rbsCn: -1}
	src := newSegDoneSource(state)
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
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

// a flags and segment write followed by a resync leaves no dirty.* field and no key with "dirty" in the namespace
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
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
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
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
	wtWaitUntil(t, "the startup purge of the dormant tombstone", func() bool { return dormantRemovedGone(e) && w.Status().PendingChanges == 0 })
	w.Stop()

	scripts := []string{}
	for _, c := range rec.recorded() {
		scripts = append(scripts, c.script)
	}
	resyncAt, purgeAt := slices.Index(scripts, FlagScript), slices.Index(scripts, PurgeScript)
	require.GreaterOrEqual(t, resyncAt, 0, "the first resync writes the flags")
	require.GreaterOrEqual(t, purgeAt, 0, "the startup purge runs")
	assert.Greater(t, purgeAt, resyncAt, "the purge runs after the first resync completes")
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

// a segment group first seen after an overflow starts frozen. Its first live write sends noCursor, and it
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
	if err := e.w.Start(src); err != nil {
		t.Fatal(err)
	}
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

// after a skipped entity and a process restart, a resync with a Source rewrites the group and releases
// the cursor. With a nil Source nothing repairs the group, so the skipped entity is never sent again.
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
			if err := e.w.Start(&wtSource{flags: wtFlags("a", "bad", "c"), flagsCn: 5}); err != nil {
				t.Fatal(err)
			}
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
			if err := w2.Start(src); err != nil {
				t.Fatal(err)
			}
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

// Same-writer case: the group freeze lives in memory, so only the instance that skipped the entity can
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
			if err := e.w.Start(src); err != nil {
				t.Fatal(err)
			}
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

// when the meta hash disappears mid-run (Redis data loss), the next write
// recreates the fingerprint and writes the cursor. Detection is out of scope; a later change that makes this write refuse
// must be a deliberate decision, so this test fails on it.
func TestFingerprintMissingMidRunRecreatedByNextWrite(t *testing.T) {
	e := newScriptEnv(t)
	w, _ := newRealWriter(t, e, wtOptions())
	src := newSegDoneSource(&wtSource{flags: wtFlags("f1"), flagsCn: 1})
	if err := w.Start(src); err != nil {
		t.Fatal(err)
	}
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

// a resync cut off mid pass leaves no cursor ahead of its data, and the restart skips only the
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
	if err := w.Start(newSource()); err != nil {
		t.Fatal(err)
	}
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
	if err := w2.Start(src2); err != nil {
		t.Fatal(err)
	}
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

// a flag that Redis holds and the source no longer lists gets a tombstone at its own change number.
func TestResyncTombstonesRedisOnlyFlag(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.flags(wtFingerprint, 7,
		ent{"a", 5, false, `{"name":"a","changeNumber":5,"status":"ACTIVE"}`},
		ent{"b", 7, false, `{"name":"b","changeNumber":7,"status":"ACTIVE"}`}))
	src := newSegDoneSource(&wtSource{flags: []dtos.SplitDTO{wtFlag("a", 5, "ACTIVE")}, flagsCn: 7})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()

	fk := e.keys.FlagKeys()
	d := e.dump()
	assert.Contains(t, d[fk[3]].Scores, "b", "b is tombstoned")
	assert.NotContains(t, d[fk[3]].Scores, "a", "a is listed by the source and stays live")
	assert.Equal(t, float64(7), d[fk[2]].Scores["b"], "b keeps its own change number")
	assert.Equal(t, float64(5), d[fk[2]].Scores["a"], "a is untouched")
}

// a rule-based segment that Redis holds and the source no longer lists gets a tombstone at its own change number.
func TestResyncTombstonesRedisOnlyRuleBasedSegment(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.rbs(wtFingerprint, 7,
		ent{"a", 5, false, `{"name":"a","changeNumber":5,"status":"ACTIVE"}`},
		ent{"b", 7, false, `{"name":"b","changeNumber":7,"status":"ACTIVE"}`}))
	src := newSegDoneSource(&wtSource{rbs: []dtos.RuleBasedSegmentDTO{{Name: "a", ChangeNumber: 5, Status: "ACTIVE"}}, rbsCn: 7})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()

	rk := e.keys.RuleBasedSegmentKeys()
	d := e.dump()
	assert.Contains(t, d[rk[3]].Scores, "b", "b is tombstoned")
	assert.NotContains(t, d[rk[3]].Scores, "a", "a is listed by the source and stays live")
	assert.Equal(t, float64(7), d[rk[2]].Scores["b"], "b keeps its own change number")
	assert.Equal(t, float64(5), d[rk[2]].Scores["a"], "a is untouched")
}

// the startup purge reads meta once, never once per PurgeScript call.
func TestPurgeReadsOnlyPurgeLastField(t *testing.T) {
	keys := wtKeys(t)
	meta := map[string]string{}
	for i := 0; i < 150; i++ {
		meta[keys.SegmentCursorField(fmt.Sprint(i))] = "1"
	}
	c := &wtClient{meta: meta}
	w, err := NewWriter(c, keys, wtFingerprint, wtOptions(), &wtLogger{})
	require.NoError(t, err)
	require.NoError(t, w.startupPurge())
	assert.Len(t, c.purgeHget, 1, "meta is read once, at the start of the purge")
	assert.Len(t, c.purgeCalls, 152, "one PurgeScript call per flag index, rule-based index and segment")
}

// Reset before Start removes the flags, rule-based segments and segment keys of an earlier run.
func TestResetBeforeStartRemovesStaleState(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.flags(wtFingerprint, 3, ent{"stale", 3, false, `{"name":"stale","changeNumber":3,"status":"ACTIVE"}`}))
	require.NoError(t, e.rbs(wtFingerprint, 3, ent{"stale", 3, false, `{"name":"stale","changeNumber":3,"status":"ACTIVE"}`}))
	require.NoError(t, e.segment(wtFingerprint, "stale", 3, []segKey{{"k", 3}}, []segKey{{"r", 3}}))
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Reset())
	src := newSegDoneSource(&wtSource{flags: []dtos.SplitDTO{wtFlag("fresh", 4, "ACTIVE")}, flagsCn: 4})
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()

	d := e.dump()
	assert.Equal(t, map[string]float64{"fresh": 4}, d[e.keys.FlagKeys()[2]].Scores, "the source flag is present, the stale one is gone")
	assert.NotContains(t, d[e.keys.FlagKeys()[3]].Scores, "stale")
	assert.NotContains(t, d[e.keys.FlagKeys()[1]].Fields, "stale", "the stale flag JSON is gone")
	assert.NotContains(t, d[e.keys.RuleBasedSegmentKeys()[2]].Scores, "stale", "the stale rule-based segment is gone")
	assert.NotContains(t, d[e.keys.RuleBasedSegmentKeys()[1]].Fields, "stale")
	sk := e.keys.SegmentKeys("stale")
	assert.Empty(t, d[sk[1]].Scores, "the stale segment has no active keys")
	assert.Empty(t, d[sk[2]].Scores, "the stale segment has no tombstones")
	assert.NotContains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("stale"))
}

// a current segment with keys Redis holds beyond the source loses them, and the next resync finds it intact.
func TestResyncSegmentExtraKeysConverges(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "s", 5, []segKey{{"a", 5}, {"b", 5}, {"x", 5}}, nil))
	seg := SegmentState{Name: "s", Active: map[string]int64{"a": 5, "b": 5}, ChangeNumber: 5}
	src := newSegDoneSource(&wtSource{segs: []SegmentState{seg}})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()
	assert.Equal(t, map[string]float64{"a": 5, "b": 5}, e.dump()[e.keys.SegmentKeys("s")[1]].Scores, "the extra key x is gone")

	cut := &cutClient{PrefixedRedisClient: e.client}
	w2, err := NewWriter(cut, e.keys, wtFingerprint, wtOptions(), &wtLogger{})
	require.NoError(t, err)
	t.Cleanup(w2.Stop)
	src2 := newSegDoneSource(&wtSource{segs: []SegmentState{seg}})
	require.NoError(t, w2.Start(src2))
	src2.wait(t)
	w2.Stop()

	checks, segWrites := 0, 0
	for _, c := range cut.recorded() {
		switch c.script {
		case SegmentCheckScript:
			checks++
		case SegmentScript:
			segWrites++
		}
	}
	assert.Equal(t, 1, checks, "the second resync checks the segment")
	assert.Zero(t, segWrites, "the second resync writes no segment")
}

// DeleteSegmentKeys removes one segment's keys and nothing else; an invalid name is refused and deletes nothing.
func TestDeleteSegmentKeysLeavesOthers(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "s1", 5, []segKey{{"a", 5}}, []segKey{{"r", 5}}))
	require.NoError(t, e.segment(wtFingerprint, "s2", 6, []segKey{{"b", 6}}, []segKey{{"q", 6}}))
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.DeleteSegmentKeys("s1"))

	s1, s2 := e.keys.SegmentKeys("s1"), e.keys.SegmentKeys("s2")
	d := e.dump()
	for _, k := range s1[1:] {
		assert.NotContains(t, d, k, "the keys of s1 are deleted")
	}
	assert.Equal(t, map[string]float64{"b": 6}, d[s2[1]].Scores)
	assert.Equal(t, map[string]float64{"q": 6}, d[s2[2]].Scores)
	assert.Contains(t, d, s2[3])
	meta := d[e.keys.Meta()].Fields
	assert.Contains(t, meta, e.keys.SegmentCursorField("s1"), "meta is not touched")
	assert.Contains(t, meta, e.keys.SegmentCursorField("s2"))

	for _, name := range []string{"s*", "", "a[b]"} {
		assert.ErrorIs(t, w.DeleteSegmentKeys(name), ErrInvalidSegmentName, name)
	}
	assert.Equal(t, map[string]float64{"b": 6}, e.dump()[s2[1]].Scores, "an invalid name deletes nothing")
}

// a key Redis holds beyond the source's members of a listed segment (same cursor, extra member) is removed by the
// resync rewrite, and the members the source lists stay.
func TestResyncRemovesStaleSegmentKeyBeyondSource(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "keep", 5, []segKey{{"a", 5}, {"zombie", 5}}, nil))
	seg := SegmentState{Name: "keep", Active: map[string]int64{"a": 5}, ChangeNumber: 5}
	src := newSegDoneSource(&wtSource{segs: []SegmentState{seg}})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()

	active := e.keys.SegmentKeys("keep")[1]
	assert.Equal(t, map[string]float64{"a": 5}, e.dump()[active].Scores, "the member Redis holds beyond the source is deleted")
}

// a segment Redis holds that the source no longer lists is dropped (keys and cursor); listed segments stay.
func TestResyncDropsRedisOnlySegment(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "keep", 5, []segKey{{"a", 5}}, nil))
	require.NoError(t, e.segment(wtFingerprint, "gone", 6, []segKey{{"b", 6}}, []segKey{{"r", 6}}))
	seg := SegmentState{Name: "keep", Active: map[string]int64{"a": 5}, ChangeNumber: 5}
	src := newSegDoneSource(&wtSource{segs: []SegmentState{seg}})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	gone := e.keys.SegmentCursorField("gone")
	require.Eventually(t, func() bool {
		_, ok := e.dump()[e.keys.Meta()].Fields[gone]
		return !ok
	}, 5*time.Second, 10*time.Millisecond, "the drop runs after the segment stream ends")
	w.Stop()

	d := e.dump()
	for _, k := range e.keys.SegmentKeys("gone")[1:] {
		assert.NotContains(t, d, k, "the keys of the unlisted segment are deleted")
	}
	meta := d[e.keys.Meta()].Fields
	assert.NotContains(t, meta, e.keys.SegmentCursorField("gone"), "its cursor is deleted")
	assert.Contains(t, meta, e.keys.SegmentCursorField("keep"))
	assert.Equal(t, map[string]float64{"a": 5}, d[e.keys.SegmentKeys("keep")[1]].Scores)
}

// a failed segment stream drops nothing, because the listed names are incomplete.
func TestResyncSegmentStreamErrorDropsNothing(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "gone", 6, []segKey{{"b", 6}}, nil))
	opts := wtOptions()
	src := newSegDoneSource(&wtSource{segsFails: 100})
	w, _ := newRealWriter(t, e, opts)
	require.NoError(t, w.Start(src))
	wtWaitUntil(t, "the stream is retried up to MaxAttempts", func() bool { return int(src.calls.Load()) >= opts.MaxAttempts })
	w.Stop()
	assert.Contains(t, e.dump(), e.keys.SegmentKeys("gone")[1], "nothing is dropped")
}

// dropFailClient is a real client whose SegmentDropScript calls fail before they reach Redis, so the keys stay in place
type dropFailClient struct {
	*redis.PrefixedRedisClient
	err   error
	drops int
}

func (c *dropFailClient) Eval(script string, keys []string, args ...interface{}) error {
	if script == SegmentDropScript {
		c.drops++
		return c.err
	}
	return c.PrefixedRedisClient.Eval(script, keys, args...)
}

// newDropFailWriter builds an unstarted Writer whose drop calls fail with err; the retry backoff is kept short
func newDropFailWriter(t *testing.T, e *scriptEnv, err error) (*Writer, *dropFailClient) {
	t.Helper()
	opts := wtOptions()
	opts.BackoffMaxWait = 10 * time.Millisecond
	client := &dropFailClient{PrefixedRedisClient: e.client, err: err}
	w, werr := NewWriter(client, e.keys, wtFingerprint, opts, &wtLogger{})
	require.NoError(t, werr)
	t.Cleanup(w.Stop)
	return w, client
}

// a transient Eval error on the drop is retried MaxAttempts times, then the drop gives up with errDropped and the
// segment keys and cursor stay in Redis
func TestDropUnlistedSegmentsTransientEvalErrorKeepsKeys(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "keep", 5, []segKey{{"a", 5}}, nil))
	require.NoError(t, e.segment(wtFingerprint, "gone", 6, []segKey{{"b", 6}}, nil))
	w, client := newDropFailWriter(t, e, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})
	fields := e.dump()[e.keys.Meta()].Fields

	err := w.dropUnlistedSegments(fields, map[string]struct{}{"keep": {}})
	assert.ErrorIs(t, err, errDropped, "the transient error runs out of tries")
	assert.Equal(t, wtOptions().MaxAttempts, client.drops, "the drop is tried MaxAttempts times")

	d := e.dump()
	assert.Contains(t, d, e.keys.SegmentKeys("gone")[1], "the keys of the unlisted segment are not deleted")
	assert.Contains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("gone"), "its cursor is not deleted")
	assert.Contains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("keep"))
}

// a permanent Eval error on the drop is not retried: the drop returns errChunkRejected and the segment keys and cursor
// stay in Redis
func TestDropUnlistedSegmentsPermanentEvalErrorKeepsKeys(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "keep", 5, []segKey{{"a", 5}}, nil))
	require.NoError(t, e.segment(wtFingerprint, "gone", 6, []segKey{{"b", 6}}, nil))
	w, client := newDropFailWriter(t, e, errors.New("WRONGTYPE Operation against a key holding the wrong kind of value"))
	fields := e.dump()[e.keys.Meta()].Fields

	err := w.dropUnlistedSegments(fields, map[string]struct{}{"keep": {}})
	assert.ErrorIs(t, err, errChunkRejected, "a permanent error is refused without retry")
	assert.Equal(t, 1, client.drops, "the drop is tried once")

	d := e.dump()
	assert.Contains(t, d, e.keys.SegmentKeys("gone")[1], "the keys of the unlisted segment are not deleted")
	assert.Contains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("gone"), "its cursor is not deleted")
}

// A dropped segment loses its in-memory group state (skipped names and whole-group freeze) together with its Redis
// keys; a segment that is still listed keeps its group state
func TestDropUnlistedSegmentsPrunesGroupState(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.segment(wtFingerprint, "keep", 5, []segKey{{"a", 5}}, nil))
	require.NoError(t, e.segment(wtFingerprint, "gone", 6, []segKey{{"b", 6}}, nil))
	w, err := NewWriter(e.client, e.keys, wtFingerprint, wtOptions(), &wtLogger{})
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	goneField := e.keys.SegmentCursorField("gone")
	keepField := e.keys.SegmentCursorField("keep")
	w.freezeOf(goneField).addSkip("b", wtOptions().MaxPendingChanges)
	w.freezeOf(keepField).addSkip("a", wtOptions().MaxPendingChanges)
	fields := e.dump()[e.keys.Meta()].Fields

	require.NoError(t, w.dropUnlistedSegments(fields, map[string]struct{}{"keep": {}}))

	assert.NotContains(t, w.groups, goneField, "the dropped segment no longer keeps group state")
	assert.Contains(t, w.groups, keepField, "a listed segment keeps its group state")
	assert.Contains(t, w.groups[keepField].names, "a")
}

// An empty source (nothing listed, no change number) is no evidence of removal, so a resync against it keeps
// the flags, rule-based segments and segments Redis holds, even when their stored JSON carries a real change number.
func TestResyncEmptySourceKeepsRedisState(t *testing.T) {
	e := newScriptEnv(t)
	require.NoError(t, e.flags(wtFingerprint, 100, ent{"a", 100, false, `{"name":"a","changeNumber":100,"status":"ACTIVE"}`}))
	require.NoError(t, e.rbs(wtFingerprint, 100, ent{"r", 100, false, `{"name":"r","changeNumber":100,"status":"ACTIVE"}`}))
	require.NoError(t, e.segment(wtFingerprint, "s", 5, []segKey{{"k", 5}}, nil))
	src := newSegDoneSource(&wtSource{flagsCn: -1, rbsCn: -1})
	w, _ := newRealWriter(t, e, wtOptions())
	require.NoError(t, w.Start(src))
	src.wait(t)
	w.Stop()

	d := e.dump()
	assert.Equal(t, float64(100), d[e.keys.FlagKeys()[2]].Scores["a"], "the flag stays in the index")
	assert.NotContains(t, d[e.keys.FlagKeys()[3]].Scores, "a", "the flag is not tombstoned")
	assert.Equal(t, float64(100), d[e.keys.RuleBasedSegmentKeys()[2]].Scores["r"])
	assert.NotContains(t, d[e.keys.RuleBasedSegmentKeys()[3]].Scores, "r")
	assert.Equal(t, map[string]float64{"k": 5}, d[e.keys.SegmentKeys("s")[1]].Scores, "the segment is not dropped")
	assert.Contains(t, d[e.keys.Meta()].Fields, e.keys.SegmentCursorField("s"))
}

// A dropped segment write freezes the segment's group; the next resync releases it and rewrites the segment even though
// Redis looks current (stored cursor and key count match the source). The source holds x where Redis holds b, so only a
// rewrite makes x appear.
func TestResyncRewritesSegmentAfterDroppedWrite(t *testing.T) {
	e := newScriptEnv(t)
	seg := SegmentState{Name: "s", Active: map[string]int64{"a": 5, "x": 6}, ChangeNumber: 6}
	require.NoError(t, e.segment(wtFingerprint, "s", 6, []segKey{{"a", 5}, {"b", 6}}, nil))
	cut := &cutClient{PrefixedRedisClient: e.client}
	opts := wtOptions()
	opts.BackoffMaxWait = 20 * time.Millisecond
	w, err := NewWriter(cut, e.keys, wtFingerprint, opts, &wtLogger{})
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	src := newSegDoneSource(&wtSource{segs: []SegmentState{seg}})
	require.NoError(t, w.Start(src))
	src.wait(t)
	require.NotContains(t, e.dump()[e.keys.SegmentKeys("s")[1]].Scores, "x", "an intact-looking segment is not rewritten")

	cut.setDown(true)
	w.PutSegment("s", []string{"c"}, nil, 7)
	wtWaitUntil(t, "the segment write used all its attempts", func() bool {
		cut.mu.Lock()
		defer cut.mu.Unlock()
		return cut.segs >= opts.MaxAttempts
	})
	cut.setDown(false)
	wtWaitUntil(t, "the resync rewrites the frozen segment", func() bool {
		return e.dump()[e.keys.SegmentKeys("s")[1]].Scores["x"] == 6
	})
}
