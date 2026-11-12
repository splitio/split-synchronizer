package redisstate

// The tests in this file need a real Redis on localhost:6379. requireRedis skips them without one,
// and fails them when CI=true.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	config "github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/dtos"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type realRedisEnv struct {
	cfg     *config.RedisConfig
	writer  *Writer
	reader  *Reader
	readCli *RedisReadClient
}

// requireRedis skips when no Redis answers on localhost:6379. With CI=true it fails instead,
// so the tests are never silently skipped.
func requireRedis(tb testing.TB) {
	tb.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DialTimeout: time.Second, MaxRetries: -1})
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := client.Ping(ctx).Err()
	if err == nil {
		return
	}
	if os.Getenv("CI") == "true" {
		tb.Fatalf("redis is required on localhost:6379 in CI: %v", err)
	}
	tb.Skipf("redis is not reachable on localhost:6379, skipping: %v", err)
}

// newRealRedisEnv starts a Writer and a Reader on a private prefix of the Redis on localhost:6379.
func newRealRedisEnv(tb testing.TB) *realRedisEnv {
	tb.Helper()
	requireRedis(tb)
	prefix := "t-" + tb.Name()
	cfg := &config.RedisConfig{Host: "localhost", Port: 6379, Database: 0, Network: "tcp", Prefix: prefix}
	logger := logging.NewLogger(nil)

	wclient, err := commonsredis.NewRedisClient(cfg, logger)
	require.NoError(tb, err)
	require.NoError(tb, wclient.Eval(ResetScript, nil, prefix+".*"))
	tb.Cleanup(func() { _ = wclient.Eval(ResetScript, nil, prefix+".*") })

	wkeys, err := NewKeys(wclient.Prefix(), "realredis-key", "")
	require.NoError(tb, err)
	w, err := NewWriter(wclient, wkeys, rdFp, DefaultOptions(), logger)
	require.NoError(tb, err)
	require.NoError(tb, w.Start(nil))
	tb.Cleanup(w.Stop)

	rc, err := NewReadClient(cfg)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = rc.Close() })
	rkeys, err := NewKeys(rc.Prefix(), "realredis-key", "")
	require.NoError(tb, err)
	r, err := NewReader(rc, rkeys, rdFp)
	require.NoError(tb, err)
	return &realRedisEnv{cfg: cfg, writer: w, reader: r, readCli: rc}
}

// drain waits until the writer has applied every queued change.
func (e *realRedisEnv) drain(tb testing.TB) {
	tb.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for e.writer.Status().PendingChanges > 0 {
		require.True(tb, time.Now().Before(deadline), "writer did not drain its queue")
		time.Sleep(5 * time.Millisecond)
	}
}

// readAll runs the Reader boot read and collects every segment through the visitor, as the boot path does.
func (e *realRedisEnv) readAll(t *testing.T) (BootState, map[string]SegmentState) {
	t.Helper()
	segs := map[string]SegmentState{}
	state, err := e.reader.Read(context.Background(), func(s SegmentState) error {
		segs[s.Name] = s
		return nil
	})
	require.NoError(t, err)
	return state, segs
}

// rawKeys lists the keys under prefix with a client that the Reader does not use.
func rawKeys(t *testing.T, prefix string) []string {
	t.Helper()
	raw := goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DialTimeout: time.Second})
	defer func() { _ = raw.Close() }()
	keys, err := raw.Keys(context.Background(), prefix+".*").Result()
	require.NoError(t, err)
	sort.Strings(keys)
	return keys
}

// TestRealRedis_RoundTrip verifies that a state written by the Writer reads back, and that its keys match
// the Keys accessors the Reader uses.
func TestRealRedis_RoundTrip(t *testing.T) {
	e := newRealRedisEnv(t)
	e.writer.PutFlags([]dtos.SplitDTO{rdFlagWithSegments("f1", 3, "s1")}, nil, 3)
	e.writer.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{{Name: "r1", Status: "ACTIVE", ChangeNumber: 4}}, nil, 4)
	e.writer.PutSegment("s1", []string{"a", "b"}, []string{"gone"}, 5)
	e.drain(t)

	state, segs := e.readAll(t)
	require.Equal(t, BootValid, state.Status, state.Reason)
	require.Len(t, state.Flags, 1)
	assert.Equal(t, "f1", state.Flags[0].Name)
	assert.Equal(t, int64(3), state.FlagCursor)
	require.Len(t, state.RuleBasedSegments, 1)
	assert.Equal(t, "r1", state.RuleBasedSegments[0].Name)
	assert.Equal(t, int64(4), state.RuleBasedSegmentCursor)
	require.Contains(t, segs, "s1")
	assert.Equal(t, map[string]int64{"a": 5, "b": 5}, segs["s1"].Active)
	assert.Equal(t, map[string]int64{"gone": 5}, segs["s1"].Removed)
	assert.Equal(t, int64(5), state.SegmentCursors["s1"])

	ctx := context.Background()
	keys, err := NewKeys(e.readCli.Prefix(), "realredis-key", "")
	require.NoError(t, err)
	for _, k := range []string{keys.FlagData(), keys.FlagIndex(), keys.RuleBasedSegmentData(), keys.SegmentActive("s1")} {
		n, err := e.readCli.client.Exists(ctx, k).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), n, "key %s written by the Writer must exist under the Keys name", k)
	}
}

// TestRealRedis_FullStateRoundTrip verifies that flags, rule-based segments, tombstones, multi-page and
// empty segments all survive a write and read in real Redis.
func TestRealRedis_FullStateRoundTrip(t *testing.T) {
	e := newRealRedisEnv(t)
	live := rdFlagWithSegments("live", 10, "big", "empty")
	live.Killed = true
	live.DefaultTreatment = "off"
	gone := dtos.SplitDTO{Name: "gone", Status: "ARCHIVED", ChangeNumber: 12}
	e.writer.PutFlags([]dtos.SplitDTO{live}, []dtos.SplitDTO{gone}, 12)
	rbsLive := dtos.RuleBasedSegmentDTO{Name: "rbs-live", Status: "ACTIVE", ChangeNumber: 20}
	rbsGone := dtos.RuleBasedSegmentDTO{Name: "rbs-gone", Status: "ARCHIVED", ChangeNumber: 21}
	e.writer.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{rbsLive}, []dtos.RuleBasedSegmentDTO{rbsGone}, 21)

	big := make([]string, 0, zscanCount*2+37)
	expectedBig := map[string]int64{}
	for i := 0; i < cap(big); i++ {
		k := fmt.Sprintf("key-%05d", i)
		big = append(big, k)
		expectedBig[k] = 9
	}
	e.writer.PutSegment("big", big, []string{"tomb-1", "tomb-2"}, 9)
	e.writer.PutSegment("empty", nil, nil, 5)
	e.drain(t)

	state, segs := e.readAll(t)
	require.Equal(t, BootValid, state.Status, state.Reason)

	assert.Equal(t, []dtos.SplitDTO{gone, live}, sortedFlags(state.Flags), "every flag field, tombstones included")
	assert.Equal(t, int64(12), state.FlagCursor)
	assert.Equal(t, []dtos.RuleBasedSegmentDTO{rbsGone, rbsLive}, sortedRBS(state.RuleBasedSegments))
	assert.Equal(t, int64(21), state.RuleBasedSegmentCursor)

	require.Contains(t, segs, "big")
	assert.Equal(t, expectedBig, segs["big"].Active, "a segment larger than one ZSCAN page is read completely")
	assert.Equal(t, map[string]int64{"tomb-1": 9, "tomb-2": 9}, segs["big"].Removed)
	assert.Equal(t, int64(9), state.SegmentCursors["big"])

	require.Contains(t, segs, "empty")
	assert.Empty(t, segs["empty"].Active)
	assert.Empty(t, segs["empty"].Removed)
	assert.Equal(t, int64(5), state.SegmentCursors["empty"], "an empty segment keeps the Writer's change number as its cursor")
	assert.Empty(t, state.SkippedSegments)
}

// TestRealRedis_KeyAndFieldDrift verifies that the key names the Writer creates match Keys exactly, and
// that the meta hash has the cursor fields the Reader reads.
func TestRealRedis_KeyAndFieldDrift(t *testing.T) {
	e := newRealRedisEnv(t)
	e.writer.PutFlags([]dtos.SplitDTO{rdFlagWithSegments("f", 1, "s1")}, []dtos.SplitDTO{{Name: "g", Status: "ARCHIVED", ChangeNumber: 2}}, 2)
	e.writer.PutRuleBasedSegments([]dtos.RuleBasedSegmentDTO{{Name: "r", Status: "ACTIVE", ChangeNumber: 3}}, []dtos.RuleBasedSegmentDTO{{Name: "rg", Status: "ARCHIVED", ChangeNumber: 3}}, 3)
	e.writer.PutSegment("s1", []string{"k1"}, []string{"t1"}, 4)
	e.writer.PutSegment("empty", nil, nil, 5)
	e.drain(t)

	keys, err := NewKeys(e.readCli.Prefix(), "realredis-key", "")
	require.NoError(t, err)

	seen := map[string]bool{}
	var expected []string
	for _, k := range append(append(keys.FlagKeys(), keys.RuleBasedSegmentKeys()...), keys.Meta(), keys.SegmentActive("s1"), keys.SegmentTombstones("s1"), keys.SegmentRemovalClock("s1"), keys.PurgeLast()) {
		if !seen[k] {
			seen[k] = true
			expected = append(expected, k)
		}
	}
	sort.Strings(expected)
	assert.Equal(t, expected, rawKeys(t, e.readCli.Prefix()), "keys in Redis must equal the names built by Keys")

	meta, err := e.readCli.HGetAll(context.Background(), keys.Meta())
	require.NoError(t, err)
	for _, field := range []string{keys.SegmentCursorField("s1"), keys.SegmentCursorField("empty"), FingerprintField} {
		assert.Contains(t, meta, field, "meta field %s written by the Writer must match the name the Reader reads", field)
	}

	state, _ := e.readAll(t)
	assert.Equal(t, BootValid, state.Status, state.Reason)
}

func sortedFlags(in []dtos.SplitDTO) []dtos.SplitDTO {
	out := append([]dtos.SplitDTO{}, in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedRBS(in []dtos.RuleBasedSegmentDTO) []dtos.RuleBasedSegmentDTO {
	out := append([]dtos.RuleBasedSegmentDTO{}, in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// rdFlagWithSegments returns an ACTIVE flag whose condition matches the given segments, so the Reader restores them.
func rdFlagWithSegments(name string, cn int64, segments ...string) dtos.SplitDTO {
	return dtos.SplitDTO{Name: name, Status: "ACTIVE", ChangeNumber: cn, Conditions: []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(segments)}}}}
}
