package redisstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rdFp = "reader-test-fp"

var errRdWrongType = errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

// rdFake is an in-memory ReadClient that records each call as "Method key".
// faults queues errors per call, consumed in order; a nil entry succeeds.
type rdFake struct {
	hashes   map[string]map[string]string
	zsets    map[string][]string
	pageSize int
	calls    []string
	faults   map[string][]error
}

func newRdFake() *rdFake {
	return &rdFake{hashes: map[string]map[string]string{}, zsets: map[string][]string{}, faults: map[string][]error{}}
}

func (f *rdFake) record(call string) error {
	f.calls = append(f.calls, call)
	queue := f.faults[call]
	if len(queue) == 0 {
		return nil
	}
	f.faults[call] = queue[1:]
	return queue[0]
}

func (f *rdFake) inject(call string, errs ...error) {
	f.faults[call] = append(f.faults[call], errs...)
}

func (f *rdFake) HGetAll(_ context.Context, key string) (map[string]string, error) {
	if err := f.record("HGetAll " + key); err != nil {
		return nil, err
	}
	out := map[string]string{}
	maps.Copy(out, f.hashes[key])
	return out, nil
}

func (f *rdFake) ZScan(_ context.Context, key string, cursor uint64, _ int64) ([]string, uint64, error) {
	if err := f.record("ZScan " + key); err != nil {
		return nil, 0, err
	}
	n := len(f.zsets[key]) / 2
	size := f.pageSize
	if size <= 0 {
		size = n
	}
	start := int(cursor)
	end := min(start+size, n)
	var next uint64
	if end < n {
		next = uint64(end)
	}
	return f.zsets[key][2*start : 2*end], next, nil
}

func (f *rdFake) ZCard(_ context.Context, key string) (int64, error) {
	if err := f.record("ZCard " + key); err != nil {
		return 0, err
	}
	return int64(len(f.zsets[key]) / 2), nil
}

func (f *rdFake) HGet(_ context.Context, key, field string) (string, bool, error) {
	if err := f.record("HGet " + key + " " + field); err != nil {
		return "", false, err
	}
	v, ok := f.hashes[key][field]
	return v, ok, nil
}

type rdFixture struct {
	t    *testing.T
	fake *rdFake
	keys Keys
}

func newRdFixture(t *testing.T) *rdFixture {
	t.Helper()
	keys, err := NewKeys("", "reader-test-key", "")
	require.NoError(t, err)
	return &rdFixture{t: t, fake: newRdFake(), keys: keys}
}

func (s *rdFixture) hash(key string, fields map[string]string) {
	if s.fake.hashes[key] == nil {
		s.fake.hashes[key] = map[string]string{}
	}
	maps.Copy(s.fake.hashes[key], fields)
}

func (s *rdFixture) scores(key string, members map[string]int64) {
	for _, m := range slices.Sorted(maps.Keys(members)) {
		s.fake.zsets[key] = append(s.fake.zsets[key], m, strconv.FormatInt(members[m], 10))
	}
}

func (s *rdFixture) reader() *Reader {
	s.t.Helper()
	r, err := NewReader(s.fake, s.keys, rdFp, withRetryPause(0))
	require.NoError(s.t, err)
	return r
}

// seedValid writes a complete state: flags f1 and f2, rule-based segment r1, segments s1 and s2.
func (s *rdFixture) seedValid() {
	k := s.keys
	s.hash(k.Meta(), map[string]string{
		FingerprintField:                rdFp,
		k.FlagCursorField():             "10",
		k.RuleBasedSegmentCursorField(): "20",
		k.SegmentCursorField("s1"):      "30",
		k.SegmentCursorField("s2"):      "31",
	})
	s.hash(k.FlagData(), map[string]string{
		"f1": rdFlagJSON(s.t, "f1", statusActive, 10, "s1"),
		"f2": rdFlagJSON(s.t, "f2", statusActive, 9),
	})
	s.scores(k.FlagIndex(), map[string]int64{"f1": 10, "f2": 9})
	s.hash(k.RuleBasedSegmentData(), map[string]string{"r1": rdRbsJSON(s.t, "r1", statusActive, 20, "s2")})
	s.scores(k.RuleBasedSegmentIndex(), map[string]int64{"r1": 20})
	s.scores(k.SegmentActive("s1"), map[string]int64{"k1": 30, "k2": 25})
	s.scores(k.SegmentTombstones("s1"), map[string]int64{"k3": 28})
	s.scores(k.SegmentActive("s2"), map[string]int64{"k9": 31})
}

// rdMatchers returns one user-defined segment condition per name. The reader restores only segments that a
// flag or rule-based segment references, so fixtures must reference the segments they expect.
func rdMatchers(refs []string) []dtos.MatcherDTO {
	out := make([]dtos.MatcherDTO, 0, len(refs))
	for _, name := range refs {
		out = append(out, dtos.MatcherDTO{UserDefinedSegment: &dtos.UserDefinedSegmentMatcherDataDTO{SegmentName: name}})
	}
	return out
}

func rdFlagJSON(t *testing.T, name, status string, cn int64, refs ...string) string {
	t.Helper()
	d := dtos.SplitDTO{Name: name, Status: status, ChangeNumber: cn}
	if len(refs) > 0 {
		d.Conditions = []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(refs)}}}
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return string(b)
}

func rdRbsJSON(t *testing.T, name, status string, cn int64, refs ...string) string {
	t.Helper()
	d := dtos.RuleBasedSegmentDTO{Name: name, Status: status, ChangeNumber: cn}
	if len(refs) > 0 {
		d.Conditions = []dtos.RuleBasedConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(refs)}}}
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return string(b)
}

func rdRead(ctx context.Context, r *Reader) (BootState, []SegmentState, error) {
	var segs []SegmentState
	state, err := r.Read(ctx, func(s SegmentState) error {
		segs = append(segs, s)
		return nil
	})
	return state, segs, err
}

func rdFlagCNs(flags []dtos.SplitDTO) map[string]int64 {
	out := map[string]int64{}
	for _, f := range flags {
		out[f.Name] = f.ChangeNumber
	}
	return out
}

func rdRbsCNs(rbs []dtos.RuleBasedSegmentDTO) map[string]int64 {
	out := map[string]int64{}
	for _, r := range rbs {
		out[r.Name] = r.ChangeNumber
	}
	return out
}

func rdExpectCorrupt(t *testing.T, s *rdFixture, reason string) {
	t.Helper()
	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.Contains(t, state.Reason, reason)
	assert.Empty(t, segs, "corrupt state must not restore any segment")
	assert.Empty(t, state.Flags)
	assert.Empty(t, state.RuleBasedSegments)
}

func TestReader_ValidState(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.seedValid()
	s.hash(k.FlagData(), map[string]string{"f3": rdFlagJSON(t, "f3", statusActive, 8)})
	s.scores(k.FlagIndex(), map[string]int64{"f3": 8})
	s.scores(k.FlagRemoved(), map[string]int64{"f3": 8})

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Empty(t, state.SkippedSegments)
	assert.Equal(t, int64(10), state.FlagCursor)
	assert.Equal(t, int64(20), state.RuleBasedSegmentCursor)
	assert.Equal(t, map[string]int64{"s1": 30, "s2": 31}, state.SegmentCursors)
	assert.Equal(t, map[string]int64{"f1": 10, "f2": 9, "f3": 8}, rdFlagCNs(state.Flags))
	assert.Equal(t, map[string]int64{"r1": 20}, rdRbsCNs(state.RuleBasedSegments))
	for _, f := range state.Flags {
		if f.Name == "f3" {
			assert.Equal(t, statusArchived, f.Status, "a removed flag is restored as archived")
		}
	}

	require.Len(t, segs, 2)
	assert.Equal(t, SegmentState{Name: "s1", Active: map[string]int64{"k1": 30, "k2": 25}, Removed: map[string]int64{"k3": 28}, ChangeNumber: 30}, segs[0])
	assert.Equal(t, SegmentState{Name: "s2", Active: map[string]int64{"k9": 31}, Removed: map[string]int64{}, ChangeNumber: 31}, segs[1])
}

func TestReader_CursorFromMetaNotMaxItem(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.hash(k.Meta(), map[string]string{FingerprintField: rdFp, k.FlagCursorField(): "5", k.SegmentCursorField("s1"): "3"})
	s.hash(k.FlagData(), map[string]string{"f1": rdFlagJSON(t, "f1", statusActive, 9, "s1")})
	s.scores(k.FlagIndex(), map[string]int64{"f1": 9})
	s.scores(k.SegmentActive("s1"), map[string]int64{"k1": 8})

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, int64(5), state.FlagCursor, "the meta cursor is the cursor, not the highest item")
	assert.Equal(t, map[string]int64{"s1": 3}, state.SegmentCursors)
	require.Len(t, segs, 1)
	assert.Equal(t, int64(3), segs[0].ChangeNumber)
	assert.Equal(t, map[string]int64{"f1": 9}, rdFlagCNs(state.Flags), "item change numbers come from the index")
}

func TestReader_Absent(t *testing.T) {
	t.Run("no meta reads nothing else", func(t *testing.T) {
		s := newRdFixture(t)
		s.hash(s.keys.FlagData(), map[string]string{"f1": rdFlagJSON(t, "f1", statusActive, 9)})
		s.scores(s.keys.FlagIndex(), map[string]int64{"f1": 9})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootAbsent, state.Status)
		assert.Empty(t, segs)
		assert.Empty(t, state.Flags)
		assert.Equal(t, []string{"HGetAll " + s.keys.Meta()}, s.fake.calls)
	})

	t.Run("meta with fingerprint and no cursors and no segments", func(t *testing.T) {
		s := newRdFixture(t)
		s.hash(s.keys.Meta(), map[string]string{FingerprintField: rdFp})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootAbsent, state.Status)
		assert.Empty(t, segs)
	})
}

func TestReader_Foreign(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.seedValid()
	s.hash(k.Meta(), map[string]string{FingerprintField: "another-environment"})

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootForeign, state.Status)
	assert.Equal(t, "fingerprint differs", state.Reason)
	assert.Empty(t, segs)
	assert.Empty(t, state.Flags)
	assert.Empty(t, state.RuleBasedSegments)
	assert.Equal(t, []string{"HGetAll " + k.Meta()}, s.fake.calls, "foreign state reads nothing but meta")
}

func TestReader_CorruptTable(t *testing.T) {
	base := func(t *testing.T) *rdFixture {
		s := newRdFixture(t)
		s.hash(s.keys.Meta(), map[string]string{FingerprintField: rdFp, s.keys.FlagCursorField(): "10"})
		return s
	}

	t.Run("bad cursor", func(t *testing.T) {
		s := base(t)
		s.hash(s.keys.Meta(), map[string]string{s.keys.FlagCursorField(): "abc"})
		rdExpectCorrupt(t, s, "does not parse")
	})

	t.Run("bad JSON", func(t *testing.T) {
		s := base(t)
		s.hash(s.keys.FlagData(), map[string]string{"f1": "{not json"})
		s.scores(s.keys.FlagIndex(), map[string]int64{"f1": 10})
		rdExpectCorrupt(t, s, "does not decode")
	})

	t.Run("hash and index mismatch", func(t *testing.T) {
		s := base(t)
		s.hash(s.keys.FlagData(), map[string]string{
			"f1": rdFlagJSON(t, "f1", statusActive, 10),
			"f2": rdFlagJSON(t, "f2", statusActive, 9),
		})
		s.scores(s.keys.FlagIndex(), map[string]int64{"f1": 10})
		rdExpectCorrupt(t, s, "data and index differ")
		assert.Equal(t, 3, rdCountCalls(s.fake, "HGetAll "+s.keys.FlagData()), "a torn read is retried, then reported corrupt")
	})

	t.Run("missing fingerprint", func(t *testing.T) {
		s := newRdFixture(t)
		s.hash(s.keys.Meta(), map[string]string{s.keys.FlagCursorField(): "10"})
		rdExpectCorrupt(t, s, "meta has no fingerprint")
	})

	t.Run("wrong type", func(t *testing.T) {
		s := base(t)
		s.fake.inject("HGetAll "+s.keys.FlagData(), errRdWrongType)
		rdExpectCorrupt(t, s, "flags data has the wrong redis type")
	})
}

func rdCountCalls(f *rdFake, call string) int {
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

func TestReader_CorruptSegmentSkippedAlone(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.fake.zsets[s.keys.SegmentActive("s1")] = []string{"k1", "1.5"}

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"s1"}, state.SkippedSegments)
	require.Len(t, segs, 1)
	assert.Equal(t, "s2", segs[0].Name)
	assert.Equal(t, map[string]int64{"s2": 31}, state.SegmentCursors)
	assert.Equal(t, map[string]int64{"f1": 10, "f2": 9}, rdFlagCNs(state.Flags), "flags still restore")
	assert.Equal(t, map[string]int64{"r1": 20}, rdRbsCNs(state.RuleBasedSegments), "rule-based segments still restore")
}

func TestReader_SegmentLargerThanOnePage(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.hash(k.Meta(), map[string]string{
		FingerprintField:                rdFp,
		k.FlagCursorField():             "10",
		k.RuleBasedSegmentCursorField(): "20",
		k.SegmentCursorField("big"):     "40",
	})
	s.hash(k.FlagData(), map[string]string{"f1": rdFlagJSON(t, "f1", statusActive, 10, "big")})
	s.scores(k.FlagIndex(), map[string]int64{"f1": 10})
	s.fake.pageSize = 2
	// Six pairs over three pages of two; k2 appears on the first and last page.
	s.fake.zsets[k.SegmentActive("big")] = []string{"k1", "40", "k2", "41", "k3", "42", "k4", "43", "k5", "44", "k2", "41"}

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Empty(t, state.SkippedSegments)
	require.Len(t, segs, 1, "the segment is visited once, after all its pages")
	assert.Equal(t, map[string]int64{"k1": 40, "k2": 41, "k3": 42, "k4": 43, "k5": 44}, segs[0].Active, "duplicates collapse")
	assert.Equal(t, 3, rdCountCalls(s.fake, "ZScan "+k.SegmentActive("big")), "every page is read")
}

func TestReader_MetaReadFirst(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.seedValid()
	// f3 is removed: its JSON change number is not compared; the index score is authoritative.
	s.hash(k.FlagData(), map[string]string{"f3": rdFlagJSON(t, "f3", statusActive, 0)})
	s.scores(k.FlagIndex(), map[string]int64{"f3": 8})
	s.scores(k.FlagRemoved(), map[string]int64{"f3": 8})

	state, _, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	require.NotEmpty(t, s.fake.calls)
	assert.Equal(t, "HGetAll "+k.Meta(), s.fake.calls[0], "meta is the first read")
	assert.Equal(t, BootValid, state.Status)

	want, err := DecodeFlag([]byte(rdFlagJSON(t, "f3", statusActive, 0)), 8, true)
	require.NoError(t, err)
	assert.Contains(t, state.Flags, want, "removed entity = data + index score + removed membership")
}

func TestReader_HashFieldNameDiffersFromJSONName(t *testing.T) {
	t.Run("flags", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.FlagData(), map[string]string{"f1": rdFlagJSON(t, "other", statusActive, 10)})
		rdExpectCorrupt(t, s, "entry name does not match its key")
	})

	t.Run("rule-based segments", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.RuleBasedSegmentData(), map[string]string{"r1": rdRbsJSON(t, "other", statusActive, 20)})
		rdExpectCorrupt(t, s, "entry name does not match its key")
	})
}

func TestReader_NonRemovedNonActiveStatus(t *testing.T) {
	for _, status := range []string{"ARCHIVED", "DRAFT"} {
		t.Run("flags "+status, func(t *testing.T) {
			s := newRdFixture(t)
			s.seedValid()
			s.hash(s.keys.FlagData(), map[string]string{"f1": rdFlagJSON(t, "f1", status, 10)})
			rdExpectCorrupt(t, s, "status is not ACTIVE")
		})
	}

	t.Run("rule-based segments", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.RuleBasedSegmentData(), map[string]string{"r1": rdRbsJSON(t, "r1", "ARCHIVED", 20)})
		rdExpectCorrupt(t, s, "status is not ACTIVE")
	})
}

func TestReader_UnsafeSegmentScores(t *testing.T) {
	for _, score := range []string{"+Inf", "-Inf", "NaN", "9007199254740992", "9007199254740993", "1e300"} {
		t.Run(score, func(t *testing.T) {
			s := newRdFixture(t)
			s.seedValid()
			s.fake.zsets[s.keys.SegmentActive("s2")] = []string{"k9", score}

			state, segs, err := rdRead(context.Background(), s.reader())
			require.NoError(t, err)
			assert.Equal(t, BootValid, state.Status)
			assert.Equal(t, []string{"s2"}, state.SkippedSegments)
			require.Len(t, segs, 1)
			assert.Equal(t, "s1", segs[0].Name)
			assert.Equal(t, map[string]int64{"f1": 10, "f2": 9}, rdFlagCNs(state.Flags))
		})
	}
}

func TestReader_MinusOneWithEntriesSkipsAlone(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "-1"})

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"s1"}, state.SkippedSegments)
	assert.Equal(t, map[string]int64{"s2": 31}, state.SegmentCursors, "the skipped segment has no cursor, so it is excluded")
	require.Len(t, segs, 1)
	assert.Equal(t, "s2", segs[0].Name)
	assert.Equal(t, map[string]int64{"f1": 10, "f2": 9}, rdFlagCNs(state.Flags))
}

func TestReader_UnparsableSegmentCursor(t *testing.T) {
	t.Run("unparsable cursor skips the segment alone", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "abc"})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		require.Len(t, segs, 1)
		assert.Equal(t, "s2", segs[0].Name)
		assert.Equal(t, map[string]int64{"f1": 10, "f2": 9}, rdFlagCNs(state.Flags))
	})

	t.Run("cursor -1 with no entries is skipped as a hold", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		delete(s.fake.zsets, s.keys.SegmentActive("s2"))
		s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s2"): "-1"})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, []string{"s2"}, state.SkippedSegments)
		assert.Contains(t, state.SkipReasons["s2"], "cursor is -1")
		require.Len(t, segs, 1)
		assert.NotContains(t, state.SegmentCursors, "s2")
	})

	t.Run("sorted set without a cursor field is not listed", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.scores(s.keys.SegmentActive("orphan"), map[string]int64{"k1": 5})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.NotContains(t, state.SegmentCursors, "orphan")
		for _, seg := range segs {
			assert.NotEqual(t, "orphan", seg.Name)
		}
		assert.NotContains(t, s.fake.calls, "ZScan "+s.keys.SegmentActive("orphan"))
	})
}

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"moved redirect", errors.New("MOVED 3999 127.0.0.1:6381"), true},
		{"ask redirect", errors.New("ASK 3999 127.0.0.1:6381"), true},
		{"word containing ASK is not transient", errors.New("TASK failed"), false},
		{"plain error", errors.New("boom"), false},
		{"loading", errors.New("LOADING Redis is loading the dataset in memory"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isTransient(tc.err))
		})
	}
}

func TestNewReader_Validation(t *testing.T) {
	var keys Keys
	t.Run("nil client returns error", func(t *testing.T) {
		r, err := NewReader(nil, keys, rdFp)
		assert.Nil(t, r)
		require.Error(t, err)
	})
	t.Run("empty fingerprint returns error", func(t *testing.T) {
		r, err := NewReader(newRdFake(), keys, "")
		assert.Nil(t, r)
		require.Error(t, err)
	})
}

func TestReader_MinusOneCursorIsSkippedWithoutScan(t *testing.T) {
	t.Run("a -1 segment is skipped", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		delete(s.fake.zsets, s.keys.SegmentActive("s1"))
		s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "-1"})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		assert.NotContains(t, s.fake.calls, "ZScan "+s.keys.SegmentActive("s1"), "a -1 cursor is skipped before any scan")
		require.Len(t, segs, 1)
	})

	t.Run("key in both sets resolves by change number and does not skip", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.scores(s.keys.SegmentTombstones("s2"), map[string]int64{"k9": 32})

		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Empty(t, state.SkippedSegments)
		require.Len(t, segs, 2)
		assert.Empty(t, segs[1].Active, "the tombstone is newer, so the active side is dropped")
		assert.Equal(t, map[string]int64{"k9": 32}, segs[1].Removed)
	})
}

func TestReader_NoUpdatedAtField(t *testing.T) {
	t.Run("field absent", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		state, _, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
	})

	t.Run("field unparsable is never read", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.Meta(), map[string]string{UpdatedAtField: "not-a-number"})
		state, _, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
	})
}

func TestReader_SegmentNamesFromMetaOnly(t *testing.T) {
	s := newRdFixture(t)
	k := s.keys
	s.seedValid()
	s.hash(k.Meta(), map[string]string{k.SegmentCursorField("empty"): "4"})
	s.hash(k.FlagData(), map[string]string{"f2": rdFlagJSON(t, "f2", statusActive, 9, "empty")})
	s.scores(k.SegmentActive("orphan"), map[string]int64{"k1": 5})

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)

	names := []string{}
	for _, seg := range segs {
		names = append(names, seg.Name)
	}
	assert.Equal(t, []string{"empty", "s1", "s2"}, names, "names come from seg.<name>.till fields only")
	require.Len(t, segs, 3)
	assert.Equal(t, SegmentState{Name: "empty", Active: map[string]int64{}, Removed: map[string]int64{}, ChangeNumber: 4}, segs[0])

	for _, field := range []string{k.FlagCursorField(), k.RuleBasedSegmentCursorField(), FingerprintField} {
		_, ok := segmentNameOfCursorField(field)
		assert.False(t, ok, "%s is not a segment field", field)
	}

	want := []string{
		"HGetAll " + k.Meta(),
		"HGetAll " + k.FlagData(), "ZScan " + k.FlagIndex(), "ZScan " + k.FlagRemoved(),
		"HGetAll " + k.RuleBasedSegmentData(), "ZScan " + k.RuleBasedSegmentIndex(), "ZScan " + k.RuleBasedSegmentRemoved(),
		"ZScan " + k.SegmentActive("empty"), "ZScan " + k.SegmentTombstones("empty"), "HGet " + k.Meta() + " " + k.SegmentCursorField("empty"),
		"ZScan " + k.SegmentActive("s1"), "ZScan " + k.SegmentTombstones("s1"), "HGet " + k.Meta() + " " + k.SegmentCursorField("s1"),
		"ZScan " + k.SegmentActive("s2"), "ZScan " + k.SegmentTombstones("s2"), "HGet " + k.Meta() + " " + k.SegmentCursorField("s2"),
	}
	assert.Equal(t, want, s.fake.calls, "no SCAN or SMEMBERS-style key discovery: only meta-named segments are read")
}

func TestReader_MinusOneFlagAndRuleBasedCursors(t *testing.T) {
	t.Run("flags -1 with no flags is valid and empty", func(t *testing.T) {
		s := newRdFixture(t)
		s.hash(s.keys.Meta(), map[string]string{
			FingerprintField:                     rdFp,
			s.keys.FlagCursorField():             "-1",
			s.keys.RuleBasedSegmentCursorField(): "4",
		})
		state, segs, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, int64(-1), state.FlagCursor)
		assert.Empty(t, state.Flags)
		assert.Empty(t, segs)
	})

	t.Run("flags -1 with flags is corrupt", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.Meta(), map[string]string{s.keys.FlagCursorField(): "-1"})
		rdExpectCorrupt(t, s, "flags stored without a cursor")
	})

	t.Run("rule-based segments -1 with no rule-based segments is valid and empty", func(t *testing.T) {
		s := newRdFixture(t)
		s.hash(s.keys.Meta(), map[string]string{
			FingerprintField:                     rdFp,
			s.keys.FlagCursorField():             "10",
			s.keys.RuleBasedSegmentCursorField(): "-1",
		})
		state, _, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, int64(-1), state.RuleBasedSegmentCursor)
		assert.Empty(t, state.RuleBasedSegments)
	})

	t.Run("rule-based segments -1 with rule-based segments is corrupt", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.hash(s.keys.Meta(), map[string]string{s.keys.RuleBasedSegmentCursorField(): "-1"})
		rdExpectCorrupt(t, s, "rule-based segments stored without a cursor")
	})
}

// hgetScript wraps rdFake and answers HGet for one field from a script, then repeats the last answer. An empty answer
// means the field is absent.
type hgetScript struct {
	*rdFake
	field   string
	answers []string
	n       int
}

func (h *hgetScript) HGet(ctx context.Context, key, field string) (string, bool, error) {
	if field != h.field {
		return h.rdFake.HGet(ctx, key, field)
	}
	i := min(h.n, len(h.answers)-1)
	h.n++
	if h.answers[i] == "" {
		return "", false, nil
	}
	return h.answers[i], true, nil
}

func (s *rdFixture) readerOn(c ReadClient) *Reader {
	s.t.Helper()
	r, err := NewReader(c, s.keys, rdFp, withRetryPause(0))
	require.NoError(s.t, err)
	return r
}

// A segment with cursor -1 is skipped with a reason and gets no cursor.
func TestReader_MinusOneCursorSkipped(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "-1"})
	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"s1"}, state.SkippedSegments)
	assert.Contains(t, state.SkipReasons["s1"], "-1")
	assert.NotContains(t, state.SegmentCursors, "s1")
	require.Len(t, segs, 1)
}

// A cursor that changes while the sets are scanned is torn: retried, then the segment is skipped.
func TestReader_CursorChangedDuringScanIsTorn(t *testing.T) {
	field := func(s *rdFixture) string { return s.keys.SegmentCursorField("s1") }

	t.Run("a change that settles is retried and restored", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		c := &hgetScript{rdFake: s.fake, field: field(s), answers: []string{"35", "35"}}
		state, _, err := rdRead(context.Background(), s.readerOn(c))
		require.NoError(t, err)
		assert.Empty(t, state.SkippedSegments)
		assert.Equal(t, int64(35), state.SegmentCursors["s1"])
		scans := 0
		for _, call := range s.fake.calls {
			if call == "ZScan "+s.keys.SegmentActive("s1") {
				scans++
			}
		}
		assert.Equal(t, 2, scans)
	})

	t.Run("a cursor that keeps moving skips the segment", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		c := &hgetScript{rdFake: s.fake, field: field(s), answers: []string{"35", "36", "37", "38"}}
		state, segs, err := rdRead(context.Background(), s.readerOn(c))
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		require.Len(t, segs, 1)
	})
}

// A cursor field removed during the scans (SegmentDropScript HDELs it) is torn like a moved cursor: retried, then the
// segment is skipped.
func TestReader_CursorDisappearedDuringScanIsTorn(t *testing.T) {
	t.Run("a field that comes back is retried and restored", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		c := &hgetScript{rdFake: s.fake, field: s.keys.SegmentCursorField("s1"), answers: []string{"", "35"}}
		state, _, err := rdRead(context.Background(), s.readerOn(c))
		require.NoError(t, err)
		assert.Empty(t, state.SkippedSegments)
		assert.Equal(t, int64(35), state.SegmentCursors["s1"])
	})

	t.Run("a field that stays gone skips the segment after every attempt", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		c := &hgetScript{rdFake: s.fake, field: s.keys.SegmentCursorField("s1"), answers: []string{""}}
		state, segs, err := rdRead(context.Background(), s.readerOn(c))
		require.NoError(t, err)
		assert.Equal(t, BootValid, state.Status)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		require.Len(t, segs, 1)
		assert.Equal(t, tornReadAttempts, c.n)
	})
}

// Removed (non-ACTIVE) flags and rule-based segments keep their stored conditions but reference no segment.
func TestReferencedSegmentNamesIgnoresRemovedEntities(t *testing.T) {
	flag := func(status string, refs ...string) dtos.SplitDTO {
		return dtos.SplitDTO{Status: status, Conditions: []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(refs)}}}}
	}
	rbs := func(status string, refs ...string) dtos.RuleBasedSegmentDTO {
		return dtos.RuleBasedSegmentDTO{Status: status, Conditions: []dtos.RuleBasedConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(refs)}}}}
	}
	got := referencedSegmentNames(
		[]dtos.SplitDTO{flag(statusActive, "live-flag-seg"), flag("ARCHIVED", "dead-flag-seg")},
		[]dtos.RuleBasedSegmentDTO{rbs(statusActive, "live-rbs-seg"), rbs("ARCHIVED", "dead-rbs-seg")},
	)
	assert.Equal(t, []string{"live-flag-seg", "live-rbs-seg"}, got)
}

// A referenced segment with no cursor in meta is skipped with a reason, not dropped.
func TestReader_ReferencedSegmentMissingFromMeta(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	delete(s.fake.hashes[s.keys.Meta()], s.keys.SegmentCursorField("s2"))
	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"s2"}, state.SkippedSegments)
	assert.Contains(t, state.SkipReasons["s2"], "no cursor in meta")
	require.Len(t, segs, 1)
}

// A negative score is corrupt for the segment, and a repeated ZSCAN cursor ends the scan.
func TestReader_NegativeScoreAndScanCycle(t *testing.T) {
	t.Run("negative score skips the segment", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		s.scores(s.keys.SegmentActive("s1"), map[string]int64{"neg": -4})
		state, _, err := rdRead(context.Background(), s.reader())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		assert.Contains(t, state.SkipReasons["s1"], "negative")
	})

	t.Run("a repeated scan cursor is corrupt", func(t *testing.T) {
		s := newRdFixture(t)
		s.seedValid()
		c := &cyclingScan{rdFake: s.fake, key: s.keys.SegmentActive("s1")}
		state, _, err := rdRead(context.Background(), s.readerOn(c))
		require.NoError(t, err)
		assert.Equal(t, []string{"s1"}, state.SkippedSegments)
		assert.Contains(t, state.SkipReasons["s1"], "twice")
	})
}

// cyclingScan answers ZSCAN for one key with the cursor 7 forever.
type cyclingScan struct {
	*rdFake
	key string
}

func (c *cyclingScan) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if key != c.key {
		return c.rdFake.ZScan(ctx, key, cursor, count)
	}
	return []string{"k", "1"}, 7, nil
}

// An exhausted budget is a typed sentinel, not a message match.
func TestReader_BudgetSentinel(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.fake.inject("HGetAll "+s.keys.Meta(), errors.New("read tcp: i/o timeout"), errors.New("read tcp: i/o timeout"))
	r, err := NewReader(s.fake, s.keys, rdFp, withRetryPause(10*time.Second), withRestoreBudget(50*time.Millisecond))
	require.NoError(t, err)
	state, err := r.Read(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.True(t, state.BudgetExhausted)

	other := newRdFixture(t)
	other.seedValid()
	other.hash(other.keys.Meta(), map[string]string{other.keys.FlagCursorField(): "-1"})
	state, err = other.reader().Read(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.False(t, state.BudgetExhausted)
}

func TestIsTransientConnectionErrors(t *testing.T) {
	for _, msg := range []string{"dial tcp: connection refused", "write: broken pipe", "EOF", "read tcp: EOF"} {
		assert.True(t, isTransient(errors.New(msg)), msg)
	}
	assert.False(t, isTransient(errors.New("WRONGTYPE bad")))
}

type rdSink struct {
	events      []string
	entityFlags int
	entityRBS   int
	segments    []SegmentState
	entitiesErr error
	segmentErr  error
}

func (k *rdSink) Entities(flags []dtos.SplitDTO, rbs []dtos.RuleBasedSegmentDTO) error {
	k.events = append(k.events, "entities")
	k.entityFlags, k.entityRBS = len(flags), len(rbs)
	return k.entitiesErr
}

func (k *rdSink) Segment(s SegmentState) error {
	k.events = append(k.events, "segment:"+s.Name)
	if k.segmentErr != nil {
		return k.segmentErr
	}
	k.segments = append(k.segments, s)
	return nil
}

// With a sink the reader hands over the entities first, then each segment, and keeps no
// segment payload in BootState.
func TestReader_SinkGetsEntitiesFirstAndStateKeepsNoPayloads(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	sink := &rdSink{}

	state, err := s.reader().ReadInto(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, BootValid, state.Status)
	assert.Equal(t, []string{"entities", "segment:s1", "segment:s2"}, sink.events)
	assert.Equal(t, 2, sink.entityFlags)
	assert.Equal(t, 1, sink.entityRBS)
	assert.Empty(t, state.Segments, "segment payloads must not be retained when a sink is given")
	assert.Equal(t, 2, state.SegmentCount)
	assert.Equal(t, map[string]int64{"s1": 30, "s2": 31}, state.SegmentCursors)
	assert.Len(t, state.Flags, 2)
	assert.Len(t, sink.segments, 2)
}

// The callback form keeps retaining segments, and SegmentCount matches.
func TestReader_CallbackFormStillRetainsSegments(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Len(t, state.Segments, 2)
	assert.Equal(t, 2, state.SegmentCount)
	assert.Len(t, segs, 2)
}

// A sink error, from the entities or from a segment, is returned and is not classified as a corrupt state.
func TestReader_SinkErrorsAreReturned(t *testing.T) {
	boom := errors.New("bolt write failed")
	for name, sink := range map[string]*rdSink{
		"entities": {entitiesErr: boom},
		"segment":  {segmentErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			s := newRdFixture(t)
			s.seedValid()
			state, err := s.reader().ReadInto(context.Background(), sink)
			require.Error(t, err)
			assert.ErrorIs(t, err, boom)
			assert.NotEqual(t, BootCorrupt, state.Status)
		})
	}
}

// Nothing is handed to the sink for a state that is not valid.
func TestReader_SinkUntouchedForInvalidState(t *testing.T) {
	s := newRdFixture(t)
	sink := &rdSink{}
	state, err := s.reader().ReadInto(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, BootAbsent, state.Status)
	assert.Empty(t, sink.events)
}

// rdOddScan overrides ZScan for one key to return an odd number of values, which rdFake cannot produce.
type rdOddScan struct {
	*rdFake
	key string
}

func (c rdOddScan) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	if key == c.key {
		return []string{"lone-member"}, 0, nil
	}
	return c.rdFake.ZScan(ctx, key, cursor, count)
}

func TestReader_OddLengthScanPageIsCorrupt(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	r, err := NewReader(rdOddScan{rdFake: s.fake, key: s.keys.FlagIndex()}, s.keys, rdFp, withRetryPause(0))
	require.NoError(t, err)
	state, segs, err := rdRead(context.Background(), r)
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.Contains(t, state.Reason, "odd number of values")
	assert.Empty(t, segs, "corrupt state must not restore any segment")
}

func TestReader_WrongTypeOnScanIsCorruptWithoutRetry(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	call := "ZScan " + s.keys.FlagIndex()
	s.fake.inject(call, errRdWrongType)
	rdExpectCorrupt(t, s, "wrong redis type")
	assert.Equal(t, 1, rdCountCalls(s.fake, call), "WRONGTYPE must not be retried")
}

func TestReader_DataEntryMissingFromIndexIsRetriedThenCorrupt(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.fake.hashes[s.keys.FlagData()] = map[string]string{
		"f1": rdFlagJSON(t, "f1", statusActive, 10, "s1"),
		"f3": rdFlagJSON(t, "f3", statusActive, 7),
	}
	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Equal(t, BootCorrupt, state.Status)
	assert.Contains(t, state.Reason, "flags")
	assert.Empty(t, segs)
	assert.Equal(t, tornReadAttempts, rdCountCalls(s.fake, "HGetAll "+s.keys.FlagData()))
}

// rdNetTimeout is a minimal net.Error for isTimeout.
type rdNetTimeout struct{ timeout bool }

func (e rdNetTimeout) Error() string   { return "rd net error" }
func (e rdNetTimeout) Timeout() bool   { return e.timeout }
func (e rdNetTimeout) Temporary() bool { return false }

func TestReader_IsTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"context deadline", context.DeadlineExceeded, true},
		{"wrapped context deadline", fmt.Errorf("read: %w", context.DeadlineExceeded), true},
		{"net timeout", rdNetTimeout{timeout: true}, true},
		{"net non-timeout", rdNetTimeout{timeout: false}, false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isTimeout(c.err))
		})
	}
}
