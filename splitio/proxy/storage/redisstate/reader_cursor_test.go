package redisstate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An interrupted multi-chunk write can leave items above the committed cursor. The reader drops them, so the restored
// state is a consistent snapshot at the cursor.

func TestReadDropsFlagsAboveCursor(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.hash(s.keys.Meta(), map[string]string{s.keys.FlagCursorField(): "9"})

	state, _, err := rdRead(context.Background(), s.reader())

	require.NoError(t, err)
	require.Equal(t, BootValid, state.Status)
	assert.Equal(t, map[string]int64{"f2": 9}, rdFlagCNs(state.Flags), "f1 at 10 is above cursor 9")
	assert.Equal(t, int64(9), state.FlagCursor)
}

func TestReadDropsRuleBasedSegmentsAboveCursor(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.hash(s.keys.Meta(), map[string]string{s.keys.RuleBasedSegmentCursorField(): "19"})

	state, segs, err := rdRead(context.Background(), s.reader())

	require.NoError(t, err)
	require.Equal(t, BootValid, state.Status)
	assert.Empty(t, state.RuleBasedSegments, "r1 at 20 is above cursor 19")
	for _, sg := range segs {
		assert.NotEqual(t, "s2", sg.Name, "s2 was referenced only by the dropped r1")
	}
}

func TestReadDropsSegmentKeysAboveCursor(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()
	s.hash(s.keys.Meta(), map[string]string{s.keys.SegmentCursorField("s1"): "29"})
	s.scores(s.keys.SegmentActive("s1"), map[string]int64{"k4": 30})
	s.scores(s.keys.SegmentTombstones("s1"), map[string]int64{"k2": 35, "k5": 29})

	state, segs, err := rdRead(context.Background(), s.reader())

	require.NoError(t, err)
	require.Equal(t, BootValid, state.Status)
	var s1 *SegmentState
	for i := range segs {
		if segs[i].Name == "s1" {
			s1 = &segs[i]
		}
	}
	require.NotNil(t, s1)
	assert.Equal(t, map[string]int64{"k2": 25}, s1.Active, "k1 (30) and k4 (30) are above 29; the k2 tombstone (35) is dropped so k2 stays active")
	assert.Equal(t, map[string]int64{"k3": 28, "k5": 29}, s1.Removed, "tombstones at or below the cursor are kept")
	for _, cn := range s1.Active {
		assert.LessOrEqual(t, cn, int64(29))
	}
	for _, cn := range s1.Removed {
		assert.LessOrEqual(t, cn, int64(29))
	}
}

func TestReadKeepsItemsAtOrBelowCursor(t *testing.T) {
	s := newRdFixture(t)
	s.seedValid()

	state, segs, err := rdRead(context.Background(), s.reader())

	require.NoError(t, err)
	assert.Len(t, state.Flags, 2)
	assert.Len(t, state.RuleBasedSegments, 1)
	assert.Len(t, segs, 2)
}
