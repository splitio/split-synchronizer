package redisstate

// The tests in this file need a real Redis on localhost:6379. requireRedis skips them without one,
// and fails them when CI=true.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	wopts := DefaultOptions()
	// PROXY_BENCH_MAX_PENDING raises the writer queue limit so a very large seed is not dropped and resynced.
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_MAX_PENDING")); err == nil && n > 0 {
		wopts.MaxPendingChanges = n
	}
	w, err := NewWriter(wclient, wkeys, rdFp, wopts, logger)
	require.NoError(tb, err)
	require.NoError(tb, w.Start(nil))
	tb.Cleanup(w.Stop)

	rc, err := NewReadClient(cfg)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = rc.Close() })
	rkeys, err := NewKeys(rc.Prefix(), "realredis-key", "")
	require.NoError(tb, err)
	var readClient ReadClient = rc
	// PROXY_BENCH_LATENCY_MS simulates a remote Redis by delaying every Reader read call.
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_LATENCY_MS")); err == nil && n > 0 {
		readClient = &latencyReadClient{inner: rc, d: time.Duration(n) * time.Millisecond}
	}
	r, err := NewReader(readClient, rkeys, rdFp)
	require.NoError(tb, err)
	return &realRedisEnv{cfg: cfg, writer: w, reader: r, readCli: rc}
}

// latencyReadClient wraps a ReadClient and sleeps d before delegating each call. Test-only.
type latencyReadClient struct {
	inner ReadClient
	d     time.Duration
}

func (l *latencyReadClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	time.Sleep(l.d)
	return l.inner.HGetAll(ctx, key)
}

func (l *latencyReadClient) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	time.Sleep(l.d)
	return l.inner.ZScan(ctx, key, cursor, count)
}

func (l *latencyReadClient) ZCard(ctx context.Context, key string) (int64, error) {
	time.Sleep(l.d)
	return l.inner.ZCard(ctx, key)
}

func (l *latencyReadClient) HGet(ctx context.Context, key, field string) (string, bool, error) {
	time.Sleep(l.d)
	return l.inner.HGet(ctx, key, field)
}

// drain waits until the writer has applied every queued change.
func (e *realRedisEnv) drain(tb testing.TB) {
	tb.Helper()
	wait := 30 * time.Second
	// PROXY_BENCH_DRAIN_SECONDS lengthens the wait for a very large benchmark seed.
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_DRAIN_SECONDS")); err == nil && n > 0 {
		wait = time.Duration(n) * time.Second
	}
	deadline := time.Now().Add(wait)
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

// TestRealRedis_RepairedSegmentIsNotTrusted is the reader side of the PR 1 R1 fix: after the writer's count mismatch
// repair deleted a segment's keys, a boot must skip that segment, never restore it empty under its old committed
// cursor. A healthy segment next to it is still restored.
func TestRealRedis_RepairedSegmentIsNotTrusted(t *testing.T) {
	e := newRealRedisEnv(t)
	e.writer.PutFlags([]dtos.SplitDTO{rdFlagWithSegments("f1", 3, "s1", "s2")}, nil, 3)
	e.writer.PutSegment("s1", []string{"a", "b"}, []string{"gone"}, 5)
	e.writer.PutSegment("s2", []string{"c"}, nil, 6)
	e.drain(t)
	e.writer.Stop()
	require.NoError(t, e.writer.DeleteSegmentKeys("s1"))

	state, segs := e.readAll(t)
	require.Equal(t, BootValid, state.Status, state.Reason)
	assert.Equal(t, []string{"s1"}, state.SkippedSegments, "the repaired segment must be skipped")
	assert.NotContains(t, segs, "s1")
	assert.NotContains(t, state.SegmentCursors, "s1", "no cursor may be trusted for the repaired segment")
	require.Contains(t, segs, "s2")
	assert.Equal(t, int64(6), state.SegmentCursors["s2"])
}

// A LIVE torn segment read. A writer rewrites a 100k-member segment generation after generation while readers boot
// the same namespace. Needs Redis on localhost:6379 (requireRedis skips without it). The window is 4 s to keep the package fast.
//
// Asserted contract, taken from the comment on Reader.fetchSegment (reader.go): "the stored cursor is <= the data (a
// writer that lands between reading meta and scanning leaves cursor <= data), and the data may be a mix of change N and
// N+1 when a writer overlaps the scans; the cursor re-check and the cardinality check detect most overlaps, the rest is
// repaired by the first sync because the cursor is never ahead of the data."
//
// Model: generation 1 seeds every key active at cn 1. Generation g >= 2 flips every key with index%5 == g%5 (active ->
// removed, removed -> active) at cn g, and the writer commits one generation at a time (cursor = g on the last chunk).
// So for a delivered segment with cursor C, key i must be in state E(i,C) (the last committed touch), or in the state
// of the in-flight touch at cn C+1, or be absent only when C+1 touches it. Anything else is not covered by the documented
// contract and is reported as a violation.

const (
	s8Members  = 100000
	s8Cohorts  = 5
	s8Segment  = "s8big"
	s8Duration = 4 * time.Second
)

func s8Key(i int) string { return fmt.Sprintf("member-%06d", i) }

// s8Touches is the number of flips key cohort c got in generations 2..g.
func s8Touches(c, g int) int {
	n := 0
	for x := 2; x <= g; x++ {
		if x%s8Cohorts == c {
			n++
		}
	}
	return n
}

// s8Expected is the state of a key of cohort c once generation g is committed: score and active flag.
func s8Expected(c, g int) (score int64, active bool) {
	last := 0
	for x := g; x >= 2; x-- {
		if x%s8Cohorts == c {
			last = x
			break
		}
	}
	n := s8Touches(c, g)
	if last == 0 {
		return 1, true
	}
	return int64(last), n%2 == 0
}

type s8Outcome struct {
	consistent  int64 // delivered, exactly the committed state E(C)
	inFlightMix int64 // delivered, with keys in the C+1 in-flight state (inside the documented contract)
	tornSkipped int64 // reader reported the segment skipped with a torn reason
	otherSkip   int64
	notValid    int64
	readErrors  int64
	violations  int64
	maxMixed    int64
}

func s8Verify(seg SegmentState, cursor int64, out *s8Outcome, samples *[]string, mu *sync.Mutex) {
	C := int(cursor)
	type exp struct {
		score  int64
		active bool
	}
	var base, flip [s8Cohorts]exp
	var flipOK [s8Cohorts]bool
	for c := 0; c < s8Cohorts; c++ {
		s, a := s8Expected(c, C)
		base[c] = exp{s, a}
		if C+1 >= 2 && (C+1)%s8Cohorts == c {
			flipOK[c] = true
			flip[c] = exp{int64(C + 1), !a}
		}
	}
	mixed := int64(0)
	bad := 0
	note := func(msg string) {
		bad++
		mu.Lock()
		if len(*samples) < 8 {
			*samples = append(*samples, msg)
		}
		mu.Unlock()
	}
	seen := 0
	for i := 0; i < s8Members; i++ {
		c := i % s8Cohorts
		k := s8Key(i)
		as, inA := seg.Active[k]
		rs, inR := seg.Removed[k]
		if inA && inR {
			note(fmt.Sprintf("C=%d key %s in both sets", C, k))
			continue
		}
		var got exp
		switch {
		case inA:
			got = exp{as, true}
			seen++
		case inR:
			got = exp{rs, false}
			seen++
		default:
			if flipOK[c] {
				mixed++ // lost by an in-flight move: the documented N/N+1 mix, repaired by replaying C+1
				continue
			}
			note(fmt.Sprintf("C=%d key %s absent from both sets and not touched by the in-flight change", C, k))
			continue
		}
		switch {
		case got == base[c]:
		case flipOK[c] && got == flip[c]:
			mixed++
		default:
			note(fmt.Sprintf("C=%d key %s got %+v, committed %+v, in-flight %+v (flip possible %v)", C, k, got, base[c], flip[c], flipOK[c]))
		}
	}
	if len(seg.Active)+len(seg.Removed) > seen {
		note(fmt.Sprintf("C=%d segment holds %d keys outside the universe", C, len(seg.Active)+len(seg.Removed)-seen))
	}
	switch {
	case bad > 0:
		atomic.AddInt64(&out.violations, 1)
	case mixed > 0:
		atomic.AddInt64(&out.inFlightMix, 1)
		for {
			old := atomic.LoadInt64(&out.maxMixed)
			if mixed <= old || atomic.CompareAndSwapInt64(&out.maxMixed, old, mixed) {
				break
			}
		}
	default:
		atomic.AddInt64(&out.consistent, 1)
	}
}

func TestLiveTornSegmentReadKeepsContract(t *testing.T) {
	e := newRealRedisEnv(t)
	keys, err := NewKeys(e.readCli.Prefix(), "realredis-key", "")
	require.NoError(t, err)

	all := make([]string, s8Members)
	for i := range all {
		all[i] = s8Key(i)
	}
	e.writer.PutFlags([]dtos.SplitDTO{rdFlagWithSegments("f1", 1, s8Segment)}, nil, 1)
	e.writer.PutSegment(s8Segment, all, nil, 1)
	seedStart := time.Now()
	e.drain(t)
	t.Logf("seeded %d members in %s", s8Members, time.Since(seedStart))

	deadline := time.Now().Add(s8Duration)
	started := time.Now()
	var gens atomic.Int64
	gens.Store(1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for g := 2; time.Now().Before(deadline); g++ {
			c := g % s8Cohorts
			n := s8Touches(c, g-1)
			var add, rem []string
			for i := c; i < s8Members; i += s8Cohorts {
				if n%2 == 0 {
					rem = append(rem, s8Key(i)) // active now: remove
				} else {
					add = append(add, s8Key(i))
				}
			}
			e.writer.PutSegment(s8Segment, add, rem, int64(g))
			for e.writer.Status().PendingChanges > 0 {
				time.Sleep(time.Millisecond)
			}
			gens.Store(int64(g))
			if g%2 == 0 {
				time.Sleep(400 * time.Millisecond) // quiet window so some boots can finish consistent
			}
		}
	}()

	var out s8Outcome
	var samples []string
	var smu sync.Mutex
	var boots atomic.Int64
	var panics atomic.Int64
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, rerr := NewReader(e.readCli, keys, rdFp)
			if rerr != nil {
				atomic.AddInt64(&out.readErrors, 1)
				return
			}
			for time.Now().Before(deadline) {
				func() {
					defer func() {
						if p := recover(); p != nil {
							panics.Add(1)
							smu.Lock()
							samples = append(samples, fmt.Sprintf("PANIC: %v", p))
							smu.Unlock()
						}
					}()
					var got *SegmentState
					state, err := reader.Read(context.Background(), func(s SegmentState) error {
						if s.Name == s8Segment {
							cp := s
							got = &cp
						}
						return nil
					})
					boots.Add(1)
					switch {
					case err != nil:
						atomic.AddInt64(&out.readErrors, 1)
						smu.Lock()
						samples = append(samples, "read error: "+err.Error())
						smu.Unlock()
					case state.Status != BootValid:
						atomic.AddInt64(&out.notValid, 1)
						smu.Lock()
						samples = append(samples, fmt.Sprintf("status %s: %s", state.Status, state.Reason))
						smu.Unlock()
					case got == nil:
						reason := state.SkipReasons[s8Segment]
						if strings.Contains(reason, "while it was read") {
							atomic.AddInt64(&out.tornSkipped, 1)
						} else {
							atomic.AddInt64(&out.otherSkip, 1)
							smu.Lock()
							samples = append(samples, "skipped, reason: "+reason)
							smu.Unlock()
						}
					default:
						if state.SegmentCursors[s8Segment] != got.ChangeNumber {
							atomic.AddInt64(&out.violations, 1)
							return
						}
						s8Verify(*got, got.ChangeNumber, &out, &samples, &smu)
					}
				}()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(started)
	e.drain(t)

	// after the writer is quiet a boot must be exactly the last committed generation
	state, segs := e.readAll(t)
	require.Equal(t, BootValid, state.Status, state.Reason)
	require.Contains(t, segs, s8Segment, "skipped: %v", state.SkipReasons)
	var final s8Outcome
	var fs []string
	var fmu sync.Mutex
	s8Verify(segs[s8Segment], segs[s8Segment].ChangeNumber, &final, &fs, &fmu)

	t.Logf("S8 elapsed=%s generations=%d boots=%d consistent=%d in_flight_mix(allowed)=%d torn_skipped=%d other_skip=%d not_valid=%d read_errors=%d violations=%d panics=%d max_mixed_keys=%d",
		elapsed.Round(time.Millisecond), gens.Load(), boots.Load(), out.consistent, out.inFlightMix, out.tornSkipped, out.otherSkip, out.notValid, out.readErrors, out.violations, panics.Load(), out.maxMixed)
	t.Logf("final quiescent boot: cursor=%d consistent=%d mix=%d violations=%d writer status=%+v", segs[s8Segment].ChangeNumber, final.consistent, final.inFlightMix, final.violations, e.writer.Status())
	for _, s := range samples {
		t.Log("sample:", s)
	}
	require.Zero(t, panics.Load(), "reader panicked")
	require.Zero(t, out.readErrors, "reader returned errors")
	require.Zero(t, out.notValid)
	require.Zero(t, out.otherSkip, "a segment was skipped for a reason other than a torn read")
	require.Zero(t, out.violations, "a delivered segment is outside the documented cursor<=data / N,N+1 contract")
	require.Equal(t, int64(1), final.consistent, "quiescent boot must match the last committed generation exactly; violations=%d", final.violations)
	require.Greater(t, boots.Load(), int64(5))
}
