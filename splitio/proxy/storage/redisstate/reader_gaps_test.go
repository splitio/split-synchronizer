package redisstate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// gapsFakeClient is an in-memory ReadClient that records every call. Methods are locked so tests can mutate state
// concurrently with a Read.
type gapsFakeClient struct {
	mu    sync.Mutex
	hash  map[string]map[string]string
	sets  map[string][]string
	calls []string
	// faults are returned in order by the next matching calls, keyed by call text. A nil entry is a success.
	faults map[string][]error
	// pages splits a sorted set into ZSCAN pages; the cursor is the page index. No pages means one page.
	pages map[string][][]string
	// scanDelay is slept before each ZSCAN; the sleep ends early when the context is done.
	scanDelay time.Duration
	// hook runs after a call is recorded, before it reads data. It runs without the lock, so it may call mutate.
	hook func(ctx context.Context, call string)
}

func newGapsFakeClient() *gapsFakeClient {
	return &gapsFakeClient{
		hash:   map[string]map[string]string{},
		sets:   map[string][]string{},
		faults: map[string][]error{},
		pages:  map[string][][]string{},
	}
}

func (c *gapsFakeClient) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

// enter records a call, returns its scripted fault and runs the hook. A done context fails the call immediately.
func (c *gapsFakeClient) enter(ctx context.Context, call string) error {
	if err := ctx.Err(); err != nil {
		c.record(call)
		return err
	}
	c.mu.Lock()
	c.calls = append(c.calls, call)
	var err error
	if q := c.faults[call]; len(q) > 0 {
		err, c.faults[call] = q[0], q[1:]
	}
	hook := c.hook
	c.mu.Unlock()
	if hook != nil {
		hook(ctx, call)
	}
	return err
}

func (c *gapsFakeClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if err := c.enter(ctx, "hgetall "+key); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.hash[key]))
	for field, value := range c.hash[key] {
		out[field] = value
	}
	return out, nil
}

func (c *gapsFakeClient) ZScan(ctx context.Context, key string, cursor uint64, _ int64) ([]string, uint64, error) {
	if err := c.enter(ctx, "zscan "+key); err != nil {
		return nil, 0, err
	}
	c.mu.Lock()
	delay := c.scanDelay
	pages := c.pages[key]
	members := slices.Clone(c.sets[key])
	c.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, 0, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if len(pages) == 0 {
		return members, 0, nil
	}
	i := int(cursor)
	if i >= len(pages) {
		return nil, 0, nil
	}
	next := uint64(i + 1)
	if i == len(pages)-1 {
		next = 0
	}
	return slices.Clone(pages[i]), next, nil
}

func (c *gapsFakeClient) ZCard(ctx context.Context, key string) (int64, error) {
	if err := c.enter(ctx, "zcard "+key); err != nil {
		return 0, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.sets[key]) / 2), nil
}

func (c *gapsFakeClient) HGet(ctx context.Context, key, field string) (string, bool, error) {
	if err := c.enter(ctx, "hget "+key+" "+field); err != nil {
		return "", false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.hash[key][field]
	return v, ok, nil
}

// callsMentioning returns recorded calls whose key contains text.
func (c *gapsFakeClient) callsMentioning(text string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, call := range c.calls {
		if strings.Contains(call, text) {
			out = append(out, call)
		}
	}
	return out
}

// countCall returns how many times exactly this call was recorded, e.g. "hgetall <key>".
func (c *gapsFakeClient) countCall(call string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, got := range c.calls {
		if got == call {
			n++
		}
	}
	return n
}

// mutate runs fn under the lock; fn may read and write c.hash and c.sets directly.
func (c *gapsFakeClient) mutate(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn()
}

const (
	gapsFlagJSON = `{"name":"flag1","status":"ACTIVE","changeNumber":5,"conditions":[{"conditionType":"ROLLOUT","matcherGroup":{"combiner":"AND","matchers":[{"matcherType":"IN_SEGMENT","userDefinedSegmentMatcherData":{"segmentName":"ref_seg"}},{"matcherType":"IN_SEGMENT","userDefinedSegmentMatcherData":{"segmentName":"ref_empty"}}]}}]}`
	gapsRBSJSON  = `{"name":"rbs1","status":"ACTIVE","changeNumber":3,"excluded":{"keys":[],"segments":[{"name":"ex_seg","type":"standard"},{"name":"large_only","type":"large"}]},"conditions":[]}`
)

// gapsState seeds one flag, one rule-based segment and four segments. Only ref_seg, ref_empty and ex_seg are
// referenced; orphan_seg and large_only have meta cursors but no references.
func gapsState(t *testing.T) (*gapsFakeClient, Keys) {
	t.Helper()
	keys, err := NewKeys("pre", "secret-api-key", "")
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	c := newGapsFakeClient()
	c.hash[keys.Meta()] = map[string]string{
		FingerprintField:                      "fp",
		keys.FlagCursorField():                "5",
		keys.RuleBasedSegmentCursorField():    "3",
		keys.SegmentCursorField("ref_seg"):    "10",
		keys.SegmentCursorField("ref_empty"):  "-1",
		keys.SegmentCursorField("ex_seg"):     "4",
		keys.SegmentCursorField("orphan_seg"): "20",
		keys.SegmentCursorField("large_only"): "21",
	}
	c.hash[keys.FlagData()] = map[string]string{"flag1": gapsFlagJSON}
	c.sets[keys.FlagIndex()] = []string{"flag1", "5"}
	c.hash[keys.RuleBasedSegmentData()] = map[string]string{"rbs1": gapsRBSJSON}
	c.sets[keys.RuleBasedSegmentIndex()] = []string{"rbs1", "3"}
	c.sets[keys.SegmentActive("ref_seg")] = []string{"k1", "1", "k2", "2"}
	c.sets[keys.SegmentActive("ex_seg")] = []string{"k3", "4"}
	c.sets[keys.SegmentActive("orphan_seg")] = []string{"k9", "3"}
	c.sets[keys.SegmentActive("large_only")] = []string{"k8", "2"}
	return c, keys
}

// gapsFlagJSONAt returns the test flag JSON with the given change number.
func gapsFlagJSONAt(cn int64) string {
	return strings.Replace(gapsFlagJSON, `"changeNumber":5`, fmt.Sprintf(`"changeNumber":%d`, cn), 1)
}

// gapsRepeat returns n errors with the same message.
func gapsRepeat(msg string, n int) []error {
	out := make([]error, n)
	for i := range out {
		out[i] = errors.New(msg)
	}
	return out
}

// gapsPages returns n ZSCAN pages of two segment entries each, with unique members and score 1.
func gapsPages(n int) [][]string {
	pages := make([][]string, n)
	for p := range pages {
		pages[p] = []string{fmt.Sprintf("m%d-a", p), "1", fmt.Sprintf("m%d-b", p), "1"}
	}
	return pages
}

// gapsReader builds a Reader on the fake with a 1ms retry pause unless opts overrides it.
func gapsReader(t *testing.T, c *gapsFakeClient, keys Keys, opts ...ReaderOption) *Reader {
	t.Helper()
	r, err := NewReader(c, keys, "fp", append([]ReaderOption{withRetryPause(time.Millisecond)}, opts...)...)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	return r
}

// gapsSegment returns the restored segment with the given name, or fails the test.
func gapsSegment(t *testing.T, state BootState, name string) SegmentState {
	t.Helper()
	for _, s := range state.Segments {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("segment %s was not restored (skipped %v)", name, state.SkippedSegments)
	return SegmentState{}
}

// gapsGeneration writes one generation of the flag and ref_seg. The caller must hold the fake lock.
func gapsGeneration(c *gapsFakeClient, keys Keys, g int64) {
	c.hash[keys.Meta()][keys.FlagCursorField()] = fmt.Sprint(g)
	c.hash[keys.Meta()][keys.SegmentCursorField("ref_seg")] = fmt.Sprint(g)
	c.hash[keys.FlagData()] = map[string]string{"flag1": gapsFlagJSONAt(g)}
	c.sets[keys.FlagIndex()] = []string{"flag1", fmt.Sprint(g)}
	c.sets[keys.SegmentActive("ref_seg")] = []string{"m", fmt.Sprint(g)}
}

func TestUnreferencedSegmentIsNotRead(t *testing.T) {
	c, keys := gapsState(t)
	r, err := NewReader(c, keys, "fp", withRetryPause(time.Millisecond))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	state, err := r.Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootValid {
		t.Fatalf("status = %s, want valid (reason %q)", state.Status, state.Reason)
	}
	for _, orphan := range []string{"orphan_seg", "large_only"} {
		if calls := c.callsMentioning(orphan); len(calls) != 0 {
			t.Errorf("unreferenced %s was read: %v", orphan, calls)
		}
		if _, ok := state.SegmentCursors[orphan]; ok {
			t.Errorf("unreferenced %s has a cursor in SegmentCursors", orphan)
		}
		for _, s := range state.Segments {
			if s.Name == orphan {
				t.Errorf("unreferenced %s was restored", orphan)
			}
		}
	}
}

func TestReferencedSegmentsAreRestored(t *testing.T) {
	c, keys := gapsState(t)
	r, err := NewReader(c, keys, "fp", withRetryPause(time.Millisecond))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	var visited []string
	state, err := r.Read(context.Background(), func(s SegmentState) error {
		visited = append(visited, s.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootValid {
		t.Fatalf("status = %s, want valid (reason %q)", state.Status, state.Reason)
	}

	wantCursors := map[string]int64{"ref_seg": 10, "ex_seg": 4}
	if len(state.SegmentCursors) != len(wantCursors) {
		t.Errorf("SegmentCursors = %v, want %v", state.SegmentCursors, wantCursors)
	}
	for name, want := range wantCursors {
		if got, ok := state.SegmentCursors[name]; !ok || got != want {
			t.Errorf("cursor of %s = %d (present %v), want %d", name, got, ok, want)
		}
	}

	var names []string
	for _, s := range state.Segments {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	slices.Sort(visited)
	wantNames := []string{"ex_seg", "ref_seg"}
	if !slices.Equal(names, wantNames) || !slices.Equal(visited, wantNames) {
		t.Errorf("restored = %v, visited = %v, want %v", names, visited, wantNames)
	}
	if !slices.Equal(state.SkippedSegments, []string{"ref_empty"}) {
		t.Errorf("skipped = %v, want [ref_empty]: cursor -1 is a hold (G-18)", state.SkippedSegments)
	}

	for _, s := range state.Segments {
		if s.Name == "ref_seg" && len(s.Active) != 2 {
			t.Errorf("ref_seg active entries = %d, want 2", len(s.Active))
		}
	}
}

// TestUnreachableRedisReturnsUnreachable verifies that Redis that stays unreachable ends as BootUnreachable
// after a bounded number of attempts.
func TestUnreachableRedisReturnsUnreachable(t *testing.T) {
	t.Run("transient errors", func(t *testing.T) {
		c, keys := gapsState(t)
		metaCall := "hgetall " + keys.Meta()
		c.faults[metaCall] = gapsRepeat("read tcp 127.0.0.1:6379: i/o timeout", 10)
		state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
		if !errors.Is(err, ErrUnreachable) || state.Status != BootUnreachable {
			t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
		}
		if n := c.countCall(metaCall); n != readAttempts {
			t.Errorf("meta read %d times, want %d", n, readAttempts)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		c, keys := gapsState(t)
		metaCall := "hgetall " + keys.Meta()
		c.faults[metaCall] = gapsRepeat("dial tcp 127.0.0.1:6379: connect: connection refused", 10)
		state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
		if !errors.Is(err, ErrUnreachable) || state.Status != BootUnreachable {
			t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
		}
		if n := c.countCall(metaCall); n < 1 || n > readAttempts {
			t.Errorf("meta read %d times, want between 1 and %d", n, readAttempts)
		}
	})
}

// A key in both the active and tombstone sets is resolved by the higher change number; a tie is a removal.
func TestKeyInBothSetsResolvedByChangeNumber(t *testing.T) {
	c, keys := gapsState(t)
	// k3: active 7 beats removed 5. k4: tie at 3, so removal. k5: removed 4 beats active 2, so removal.
	c.sets[keys.SegmentActive("ref_seg")] = []string{"k1", "1", "k2", "2", "k3", "7", "k4", "3", "k5", "2"}
	c.sets[keys.SegmentTombstones("ref_seg")] = []string{"k3", "5", "k4", "3", "k5", "4"}

	var visited []string
	state, err := gapsReader(t, c, keys).Read(context.Background(), func(s SegmentState) error {
		visited = append(visited, s.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootValid {
		t.Fatalf("status = %s, want valid (reason %q)", state.Status, state.Reason)
	}
	if !slices.Equal(state.SkippedSegments, []string{"ref_empty"}) {
		t.Errorf("skipped = %v, want [ref_empty] (cursor -1)", state.SkippedSegments)
	}
	slices.Sort(visited)
	if want := []string{"ex_seg", "ref_seg"}; !slices.Equal(visited, want) {
		t.Errorf("visited = %v, want %v", visited, want)
	}
	seg := gapsSegment(t, state, "ref_seg")
	if want := map[string]int64{"k1": 1, "k2": 2, "k3": 7}; !maps.Equal(seg.Active, want) {
		t.Errorf("ref_seg active = %v, want %v", seg.Active, want)
	}
	if want := map[string]int64{"k4": 3, "k5": 4}; !maps.Equal(seg.Removed, want) {
		t.Errorf("ref_seg removed = %v, want %v", seg.Removed, want)
	}
}

// TestManyPageScanIsNotCutByBudget verifies that the restore budget bounds the whole read, not each ZSCAN.
// The per-call timeout is a 30s constant, so stalls are simulated with timeout errors.
func TestManyPageScanIsNotCutByBudget(t *testing.T) {
	c, keys := gapsState(t)
	c.pages[keys.SegmentActive("ref_seg")] = gapsPages(50)
	c.scanDelay = time.Millisecond

	state, err := gapsReader(t, c, keys, withRestoreBudget(10*time.Second)).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootValid {
		t.Fatalf("status = %s (reason %q), want valid", state.Status, state.Reason)
	}
	if seg := gapsSegment(t, state, "ref_seg"); len(seg.Active) != 100 {
		t.Errorf("ref_seg active = %d entries, want 100", len(seg.Active))
	}
	if n := c.countCall("zscan " + keys.SegmentActive("ref_seg")); n != 50 {
		t.Errorf("ZSCAN pages = %d, want 50", n)
	}
}

func TestStalledCallIsUnreachable(t *testing.T) {
	c, keys := gapsState(t)
	flagIndexCall := "zscan " + keys.FlagIndex()
	c.faults[flagIndexCall] = []error{context.DeadlineExceeded, context.DeadlineExceeded}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if !errors.Is(err, ErrUnreachable) || state.Status != BootUnreachable {
		t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
	}
	if n := c.countCall(flagIndexCall); n != 2 {
		t.Errorf("stalled ZSCAN called %d times, want 2 (one retry of a timed-out call)", n)
	}
}

func TestExhaustedBudgetEndsCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	segCall := "zscan " + keys.SegmentActive("ref_seg")
	// The hook holds the segment ZSCAN until the budget expires. The 5s timer only stops a broken build from hanging.
	c.hook = func(ctx context.Context, call string) {
		if call != segCall {
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}

	state, err := gapsReader(t, c, keys, withRestoreBudget(30*time.Millisecond)).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if !strings.Contains(state.Reason, "budget") {
		t.Errorf("reason %q does not mention the restore budget", state.Reason)
	}
}

// A torn flags read (index ahead of data) is retried until it is consistent.
func TestTornFlagsReadRetriedThenValid(t *testing.T) {
	c, keys := gapsState(t)
	dataCall := "hgetall " + keys.FlagData()
	// Index at change 6, data at 5: the first read is torn.
	c.sets[keys.FlagIndex()] = []string{"flag1", "6"}
	dataReads := 0
	c.hook = func(_ context.Context, call string) {
		if call != dataCall {
			return
		}
		dataReads++
		if dataReads == 2 {
			c.mutate(func() { c.hash[keys.FlagData()] = map[string]string{"flag1": gapsFlagJSONAt(6)} })
		}
	}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootValid {
		t.Fatalf("status = %s (reason %q), want valid", state.Status, state.Reason)
	}
	if len(state.Flags) != 1 || state.Flags[0].ChangeNumber != 6 {
		t.Errorf("flags = %+v, want one flag at change number 6", state.Flags)
	}
	if n := c.countCall(dataCall); n != 2 {
		t.Errorf("flag data read %d times, want 2", n)
	}
}

func TestPermanentlyTornFlagsReadIsCorruptAfterThreeReads(t *testing.T) {
	c, keys := gapsState(t)
	c.sets[keys.FlagIndex()] = []string{"flag1", "6"}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if !strings.Contains(state.Reason, "flags") {
		t.Errorf("reason %q does not name the flags read", state.Reason)
	}
	if n := c.countCall("hgetall " + keys.FlagData()); n != tornReadAttempts {
		t.Errorf("flag data read %d times, want %d", n, tornReadAttempts)
	}
	if n := c.countCall("zscan " + keys.FlagIndex()); n != tornReadAttempts {
		t.Errorf("flag index read %d times, want %d", n, tornReadAttempts)
	}
}

func TestBadFlagJSONIsNotRetried(t *testing.T) {
	c, keys := gapsState(t)
	c.hash[keys.FlagData()] = map[string]string{"flag1": `{"name":"flag1"`}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if !strings.Contains(state.Reason, "does not decode") {
		t.Errorf("reason %q does not report a decode failure", state.Reason)
	}
	if n := c.countCall("hgetall " + keys.FlagData()); n != 1 {
		t.Errorf("flag data read %d times, want 1 (decode errors are not torn)", n)
	}
}

// TestTransientErrorsAreRetriedBounded verifies that transient errors are retried a bounded number of times,
// then the read succeeds or ends Unreachable.
func TestTransientErrorsAreRetriedBounded(t *testing.T) {
	msgs := []string{
		"read tcp 127.0.0.1:6379: connection reset by peer",
		"read tcp 127.0.0.1:6379: i/o timeout",
		"LOADING Redis is loading the dataset in memory",
		"MOVED 3999 127.0.0.1:6381",
		"ASK 3999 127.0.0.1:6381",
		"TRYAGAIN Multiple keys request during rehashing",
	}
	for _, msg := range msgs {
		t.Run("recovers after "+msg, func(t *testing.T) {
			c, keys := gapsState(t)
			metaCall := "hgetall " + keys.Meta()
			c.faults[metaCall] = gapsRepeat(msg, readAttempts-1)
			state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
			if err != nil || state.Status != BootValid {
				t.Fatalf("status = %s, err = %v, want valid", state.Status, err)
			}
			if n := c.countCall(metaCall); n != readAttempts {
				t.Errorf("meta read %d times, want %d", n, readAttempts)
			}
		})
		t.Run("gives up on "+msg, func(t *testing.T) {
			c, keys := gapsState(t)
			metaCall := "hgetall " + keys.Meta()
			c.faults[metaCall] = gapsRepeat(msg, 10)
			state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
			if !errors.Is(err, ErrUnreachable) || state.Status != BootUnreachable {
				t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
			}
			if n := c.countCall(metaCall); n != readAttempts {
				t.Errorf("meta read %d times, want %d", n, readAttempts)
			}
		})
	}
}

// Decode errors, WRONGTYPE and bad cursors are corrupt and are not retried.
func TestNonTransientErrorsAreCorruptWithoutRetry(t *testing.T) {
	t.Run("WRONGTYPE on flag data", func(t *testing.T) {
		c, keys := gapsState(t)
		dataCall := "hgetall " + keys.FlagData()
		c.faults[dataCall] = []error{errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")}
		state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
		if err != nil || state.Status != BootCorrupt {
			t.Fatalf("status = %s, err = %v, want corrupt", state.Status, err)
		}
		if n := c.countCall(dataCall); n != 1 {
			t.Errorf("flag data read %d times, want 1", n)
		}
	})

	t.Run("bad cursor", func(t *testing.T) {
		c, keys := gapsState(t)
		c.hash[keys.Meta()][keys.FlagCursorField()] = "abc"
		state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
		if err != nil || state.Status != BootCorrupt {
			t.Fatalf("status = %s, err = %v, want corrupt", state.Status, err)
		}
		if n := c.countCall("hgetall " + keys.Meta()); n != 1 {
			t.Errorf("meta read %d times, want 1", n)
		}
		if calls := c.callsMentioning("zscan"); len(calls) != 0 {
			t.Errorf("state was read after a bad cursor: %v", calls)
		}
	})

	t.Run("decode error on rule-based segment", func(t *testing.T) {
		c, keys := gapsState(t)
		rbsCall := "hgetall " + keys.RuleBasedSegmentData()
		c.hash[keys.RuleBasedSegmentData()] = map[string]string{"rbs1": `{"name":`}
		state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
		if err != nil || state.Status != BootCorrupt {
			t.Fatalf("status = %s, err = %v, want corrupt", state.Status, err)
		}
		if n := c.countCall(rbsCall); n != 1 {
			t.Errorf("rule-based segment data read %d times, want 1", n)
		}
	})
}

// An exhausted budget during a retry pause ends the read as Corrupt without sleeping the full pause.
func TestBudgetExhaustedDuringRetryIsCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	metaCall := "hgetall " + keys.Meta()
	c.faults[metaCall] = gapsRepeat("read tcp 127.0.0.1:6379: i/o timeout", 10)

	start := time.Now()
	state, err := gapsReader(t, c, keys, withRetryPause(10*time.Second), withRestoreBudget(50*time.Millisecond)).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("read took %v, the retry pause should have been cut by the budget", elapsed)
	}
	if n := c.countCall(metaCall); n != 1 {
		t.Errorf("meta read %d times, want 1 before the pause was cut", n)
	}
}

// A rule-based segment whose JSON change number differs from its index score is retried, then Corrupt.
func TestNonRemovedChangeNumberMismatchIsTornThenCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	rbsDataCall := "hgetall " + keys.RuleBasedSegmentData()
	rbsIndexCall := "zscan " + keys.RuleBasedSegmentIndex()
	// JSON says change number 3, the index says 4.
	c.sets[keys.RuleBasedSegmentIndex()] = []string{"rbs1", "4"}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if !strings.Contains(state.Reason, "change number differs") {
		t.Errorf("reason = %q, want the change number mismatch", state.Reason)
	}
	if n := c.countCall(rbsDataCall); n != tornReadAttempts {
		t.Errorf("rule-based segment data read %d times, want %d", n, tornReadAttempts)
	}
	if n := c.countCall(rbsIndexCall); n != tornReadAttempts {
		t.Errorf("rule-based segment index read %d times, want %d", n, tornReadAttempts)
	}
}

// With a concurrent monotonic writer (run with -race), a read is Valid or Corrupt, never a mixed view.
func TestConcurrentWriterGivesValidOrCorruptNeverMixed(t *testing.T) {
	c, keys := gapsState(t)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for g := int64(6); ; g++ {
			select {
			case <-stop:
				return
			default:
			}
			c.mutate(func() { gapsGeneration(c, keys, g) })
		}
	}()

	r := gapsReader(t, c, keys)
	const reads = 40
	valid, corrupt := 0, 0
	for i := 0; i < reads; i++ {
		state, err := r.Read(context.Background(), nil)
		if err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("Read: %v", err)
		}
		switch state.Status {
		case BootValid:
			valid++
			if len(state.Flags) != 1 {
				close(stop)
				wg.Wait()
				t.Fatalf("valid state has %d flags, want 1", len(state.Flags))
			}
			if state.FlagCursor > state.Flags[0].ChangeNumber {
				t.Errorf("flag cursor %d is newer than flag data %d", state.FlagCursor, state.Flags[0].ChangeNumber)
			}
			if slices.Contains(state.SkippedSegments, "ref_seg") {
				continue
			}
			seg := gapsSegment(t, state, "ref_seg")
			var newest int64
			for _, cn := range seg.Active {
				newest = max(newest, cn)
			}
			for _, cn := range seg.Removed {
				newest = max(newest, cn)
			}
			if state.SegmentCursors["ref_seg"] > newest {
				t.Errorf("ref_seg cursor %d is newer than its data %d", state.SegmentCursors["ref_seg"], newest)
			}
		case BootCorrupt:
			corrupt++
		default:
			close(stop)
			wg.Wait()
			t.Fatalf("status = %s (reason %q), want valid or corrupt", state.Status, state.Reason)
		}
	}
	close(stop)
	wg.Wait()
	t.Logf("valid = %d, corrupt = %d", valid, corrupt)
	if valid+corrupt != reads {
		t.Errorf("valid+corrupt = %d, want %d reads", valid+corrupt, reads)
	}
	if valid == 0 {
		t.Errorf("valid = 0 across %d reads, want at least one valid read", reads)
	}
}

// A caller cancel during a retry pause is unreachable, not a spent restore budget.
func TestCancelDuringRetryPauseIsUnreachable(t *testing.T) {
	c, keys := gapsState(t)
	metaCall := "hgetall " + keys.Meta()
	c.faults[metaCall] = gapsRepeat("read tcp 127.0.0.1:6379: i/o timeout", 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)

	start := time.Now()
	state, err := gapsReader(t, c, keys, withRetryPause(10*time.Second)).Read(ctx, nil)
	if state.Status != BootUnreachable || !errors.Is(err, ErrUnreachable) {
		t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
	}
	if state.BudgetExhausted {
		t.Errorf("caller cancel reported as an exhausted budget")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("read took %v, the retry pause should have been cut by the cancel", elapsed)
	}
}

// An expired restore budget during a retry pause is corrupt with BudgetExhausted set and no error.
func TestBudgetExpiryDuringRetryPauseIsCorruptWithBudgetFlag(t *testing.T) {
	c, keys := gapsState(t)
	metaCall := "hgetall " + keys.Meta()
	c.faults[metaCall] = gapsRepeat("read tcp 127.0.0.1:6379: i/o timeout", 10)

	state, err := gapsReader(t, c, keys, withRetryPause(10*time.Second), withRestoreBudget(30*time.Millisecond)).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt || !state.BudgetExhausted {
		t.Fatalf("status = %s, BudgetExhausted = %v (reason %q), want corrupt with budget exhausted", state.Status, state.BudgetExhausted, state.Reason)
	}
}

// A caller cancel during the torn-read retry pause is unreachable, not a spent restore budget.
func TestCancelDuringTornReadPauseIsUnreachable(t *testing.T) {
	c, keys := gapsState(t)
	c.sets[keys.FlagIndex()] = []string{"flag1", "6"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)

	state, err := gapsReader(t, c, keys, withRetryPause(10*time.Second)).Read(ctx, nil)
	if state.Status != BootUnreachable || !errors.Is(err, ErrUnreachable) {
		t.Fatalf("status = %s, err = %v, want unreachable", state.Status, err)
	}
	if state.BudgetExhausted {
		t.Errorf("caller cancel reported as an exhausted budget")
	}
}

// A flag cursor below the no-cursor value is corrupt for the whole state, even with a valid flag present.
func TestCursorBelowNoCursorIsCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	c.hash[keys.Meta()][keys.FlagCursorField()] = "-7"

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if state.Reason == "" {
		t.Errorf("corrupt state has an empty reason")
	}
}

// A hostile cursor value is truncated in the corrupt reason, never logged in full.
func TestHostileCursorValueIsTruncatedInReason(t *testing.T) {
	c, keys := gapsState(t)
	hostile := strings.Repeat("x", 500)
	c.hash[keys.Meta()][keys.FlagCursorField()] = hostile

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if len(state.Reason) >= 200 {
		t.Errorf("reason has %d bytes, want < 200", len(state.Reason))
	}
	if strings.Contains(state.Reason, hostile) {
		t.Errorf("reason embeds the full 500-character cursor value")
	}
}

// A flag whose key name is empty is corrupt, even when its JSON name is also empty.
func TestEmptyEntityNameIsCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	c.sets[keys.FlagIndex()] = []string{"", "5"}
	c.hash[keys.FlagData()] = map[string]string{"": strings.Replace(gapsFlagJSON, `"name":"flag1"`, `"name":""`, 1)}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if state.Reason == "" {
		t.Errorf("corrupt state has an empty reason")
	}
}

// A removed flag that is absent from the index is a torn read: data and index agree in size, but the
// removed set names a flag the index does not hold.
func TestReader_TornRemovedEntryNotIndexedRetriedThenCorrupt(t *testing.T) {
	c, keys := gapsState(t)
	c.sets[keys.FlagRemoved()] = []string{"flag2", "6"}

	state, err := gapsReader(t, c, keys).Read(context.Background(), nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != BootCorrupt {
		t.Fatalf("status = %s (reason %q), want corrupt", state.Status, state.Reason)
	}
	if !strings.Contains(state.Reason, "removed entry is not indexed") {
		t.Errorf("reason %q does not report the torn removed entry", state.Reason)
	}
	if n := c.countCall("hgetall " + keys.FlagData()); n != tornReadAttempts {
		t.Errorf("flag data read %d times, want %d", n, tornReadAttempts)
	}
}
