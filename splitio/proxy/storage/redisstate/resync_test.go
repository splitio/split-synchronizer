package redisstate

import (
	"errors"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
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
			if c.args[3] != e.w.keys.FlagCursorField() || wtHasArg(c.args, "bad") {
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
