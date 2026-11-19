package redisstate

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rdZCardFunc overrides ZCard of rdFake with a custom answer and leaves every other call to the fake.
type rdZCardFunc struct {
	*rdFake
	zcard func(ctx context.Context, key string) (int64, error)
}

func (f *rdZCardFunc) ZCard(ctx context.Context, key string) (int64, error) {
	return f.zcard(ctx, key)
}

// rdCardFake answers ZCard for one key from a queue; the last answer repeats. Other keys use the fake's data.
type rdCardFake struct {
	*rdFake
	key     string
	answers []int64
}

func (f *rdCardFake) ZCard(ctx context.Context, key string) (int64, error) {
	if key != f.key || len(f.answers) == 0 {
		return f.rdFake.ZCard(ctx, key)
	}
	if err := f.record("ZCard " + key); err != nil {
		return 0, err
	}
	v := f.answers[0]
	if len(f.answers) > 1 {
		f.answers = f.answers[1:]
	}
	return v, nil
}

// F4-04: the presize ZCARD must run under readCall, so it has a per call deadline.
func TestScanScoresZCardUsesReadCallTimeout(t *testing.T) {
	t.Run("ZCARD runs under a per call deadline and a failure leaves the scan intact", func(t *testing.T) {
		s := newRdFixture(t)
		key := s.keys.SegmentActive("s1")
		s.scores(key, map[string]int64{"k1": 1, "k2": 2})
		s.fake.pageSize = 1

		var hadDeadline bool
		c := &rdZCardFunc{rdFake: s.fake, zcard: func(ctx context.Context, _ string) (int64, error) {
			_, hadDeadline = ctx.Deadline()
			return 0, errors.New("zcard unavailable")
		}}

		out, err := s.readerOn(c).scanScores(context.Background(), key, "segment")
		require.NoError(t, err)
		assert.True(t, hadDeadline, "ZCARD must run with a per call deadline")
		assert.Equal(t, map[string]int64{"k1": 1, "k2": 2}, out)
	})

	t.Run("failing ZCARD does not fail the scan", func(t *testing.T) {
		s := newRdFixture(t)
		key := s.keys.SegmentActive("s1")
		s.scores(key, map[string]int64{"k1": 1, "k2": 2})
		s.fake.pageSize = 1

		c := &rdZCardFunc{rdFake: s.fake, zcard: func(context.Context, string) (int64, error) {
			return 0, errors.New("ERR boom")
		}}

		out, err := s.readerOn(c).scanScores(context.Background(), key, "segment")
		require.NoError(t, err)
		assert.Equal(t, map[string]int64{"k1": 1, "k2": 2}, out)
	})
}

// seedSegmentS1 seeds one segment s1 with two active members, referenced by flag f1.
func seedSegmentS1(t *testing.T, s *rdFixture) {
	t.Helper()
	k := s.keys
	s.hash(k.Meta(), map[string]string{FingerprintField: rdFp, k.FlagCursorField(): "5", k.SegmentCursorField("s1"): "3"})
	s.hash(k.FlagData(), map[string]string{"f1": rdFlagJSON(t, "f1", statusActive, 9, "s1")})
	s.scores(k.FlagIndex(), map[string]int64{"f1": 9})
	s.scores(k.SegmentActive("s1"), map[string]int64{"k1": 2, "k2": 1})
}

// F4-01: a segment whose ZCARD differs from the scanned size is torn and must not be restored from that read.
func TestFetchSegmentTornWhenCardinalityChangesDuringScan(t *testing.T) {
	s := newRdFixture(t)
	seedSegmentS1(t, s)
	c := &rdCardFake{rdFake: s.fake, key: s.keys.SegmentActive("s1"), answers: []int64{3}}

	state, segs, err := rdRead(context.Background(), s.readerOn(c))
	require.NoError(t, err)
	assert.Empty(t, segs)
	assert.Equal(t, []string{"s1"}, state.SkippedSegments)
	assert.Contains(t, state.SkipReasons["s1"], "changed size")
	assert.Equal(t, tornReadAttempts, rdCountCalls(s.fake, "ZCard "+s.keys.SegmentActive("s1")))
}

func TestFetchSegmentTornThenStableIsRestored(t *testing.T) {
	s := newRdFixture(t)
	seedSegmentS1(t, s)
	c := &rdCardFake{rdFake: s.fake, key: s.keys.SegmentActive("s1"), answers: []int64{3, 2}}

	state, segs, err := rdRead(context.Background(), s.readerOn(c))
	require.NoError(t, err)
	assert.Empty(t, state.SkippedSegments)
	require.Len(t, segs, 1)
	assert.Equal(t, SegmentState{Name: "s1", Active: map[string]int64{"k1": 2, "k2": 1}, Removed: map[string]int64{}, ChangeNumber: 3}, segs[0])
	assert.Equal(t, 2, rdCountCalls(s.fake, "ZCard "+s.keys.SegmentActive("s1")))
}

func TestFetchSegmentCardinalityMatchesRestores(t *testing.T) {
	s := newRdFixture(t)
	seedSegmentS1(t, s)

	state, segs, err := rdRead(context.Background(), s.reader())
	require.NoError(t, err)
	assert.Empty(t, state.SkippedSegments)
	require.Len(t, segs, 1)
	assert.Equal(t, int64(3), segs[0].ChangeNumber)
	assert.Equal(t, 1, rdCountCalls(s.fake, "ZCard "+s.keys.SegmentActive("s1")))
}
