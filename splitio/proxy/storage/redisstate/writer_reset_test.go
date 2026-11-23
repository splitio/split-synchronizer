package redisstate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const TestResetFForeignFP = "fp-foreign"

// TestResetFEnv is one environment namespace on a real redis-server, with its lease seeded to testOwner at testToken.
type TestResetFEnv struct {
	srv    *ftServer
	client Client
	keys   Keys
	log    *wtLogger
}

func (e *TestResetFEnv) open(t *testing.T, srv *ftServer, prefix string) {
	t.Helper()
	e.srv = srv
	e.log = &wtLogger{}
	client, err := commonsredis.NewRedisClient(srv.cfg(prefix), e.log)
	require.NoError(t, err)
	e.client = client
	e.keys, err = NewKeys(client.Prefix(), "ft-key", "")
	require.NoError(t, err)
}

// seed sets the lease, the given meta fields and the given data keys
func (e *TestResetFEnv) seed(t *testing.T, fields map[string]interface{}, data []string) {
	t.Helper()
	ctx := context.Background()
	ftSeedLease(t, e.srv, e.keys)
	if len(fields) > 0 {
		require.NoError(t, e.srv.admin.HSet(ctx, e.keys.Meta(), fields).Err())
	}
	_, err := e.srv.admin.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for _, k := range data {
			p.Set(ctx, k, "x", 0)
		}
		return nil
	})
	require.NoError(t, err)
}

func (e *TestResetFEnv) writer(t *testing.T, c Client, token int64) *Writer {
	t.Helper()
	opts := ftOpts()
	opts.Token = token
	w, err := NewWriter(c, e.keys, wtFingerprint, opts, e.log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return w
}

func (e *TestResetFEnv) meta(t *testing.T) map[string]string {
	t.Helper()
	m, err := e.srv.admin.HGetAll(context.Background(), e.keys.Meta()).Result()
	require.NoError(t, err)
	return m
}

func (e *TestResetFEnv) allKeys(t *testing.T) []string {
	t.Helper()
	ks, err := e.srv.admin.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	return ks
}

func (e *TestResetFEnv) existing(t *testing.T, keys ...string) int64 {
	t.Helper()
	n, err := e.srv.admin.Exists(context.Background(), keys...).Result()
	require.NoError(t, err)
	return n
}

func (e *TestResetFEnv) fixedKeys() []string {
	k := e.keys
	return []string{
		k.PurgeLast(), k.FlagData(), k.FlagIndex(), k.FlagRemoved(),
		k.RuleBasedSegmentData(), k.RuleBasedSegmentIndex(), k.RuleBasedSegmentRemoved(),
	}
}

func (e *TestResetFEnv) segmentNames(n int) []string {
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		names = append(names, fmt.Sprintf("s%03d", i))
	}
	return names
}

// segmentFields returns the meta fields of the given segments: a cursor each
func (e *TestResetFEnv) segmentFields(names []string) map[string]interface{} {
	fields := map[string]interface{}{"fingerprint": wtFingerprint, "flags.till": 5, "rbs.till": 5}
	for _, name := range names {
		fields[e.keys.SegmentCursorField(name)] = 5
	}
	return fields
}

// segmentData returns the three data keys of each segment
func (e *TestResetFEnv) segmentData(names []string) []string {
	var data []string
	for _, name := range names {
		data = append(data, e.keys.SegmentKeys(name)[1:]...)
	}
	return data
}

func (e *TestResetFEnv) metaFields(meta map[string]string) []string {
	var fields []string
	for f := range meta {
		fields = append(fields, f)
	}
	return fields
}

// TestResetFSpyClient wraps a real client. It records every ResetScript 'del' call (KEYS[3..] and the ARGV cursor
// fields) and fails the failDel-th one (1-based, 0 means never) without sending it.
type TestResetFSpyClient struct {
	Client
	failDel int
	mu      sync.Mutex
	dels    []TestResetFDelCall
	modes   []string
}

type TestResetFDelCall struct {
	keys    []string
	cursors []string
}

func (c *TestResetFSpyClient) Eval(script string, keys []string, args ...interface{}) error {
	if script != ResetScript || len(args) < 3 {
		return c.Client.Eval(script, keys, args...)
	}
	mode := fmt.Sprint(args[2])
	c.mu.Lock()
	c.modes = append(c.modes, mode)
	c.mu.Unlock()
	if mode != "del" {
		return c.Client.Eval(script, keys, args...)
	}
	call := TestResetFDelCall{keys: append([]string(nil), keys[2:]...)}
	for _, a := range args[3:] {
		call.cursors = append(call.cursors, fmt.Sprint(a))
	}
	c.mu.Lock()
	c.dels = append(c.dels, call)
	fail := c.failDel > 0 && len(c.dels) == c.failDel
	c.mu.Unlock()
	if fail {
		return errors.New("injected del failure")
	}
	return c.Client.Eval(script, keys, args...)
}

func (c *TestResetFSpyClient) calls() []TestResetFDelCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TestResetFDelCall(nil), c.dels...)
}

// order returns the mode (args[2]) of every ResetScript call, in call order
func (c *TestResetFSpyClient) order() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.modes...)
}

// F-1: a foreign fingerprint with a valid lease and fence is taken over by Reset, and the writer then writes normally
func TestResetFTakesOverForeignFingerprint(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-a")
	data := []string{env.keys.FlagData(), env.keys.FlagIndex(), env.keys.SegmentActive("s1"), env.keys.SegmentTombstones("s1"), env.keys.SegmentRemovalClock("s1")}
	env.seed(t, map[string]interface{}{"fingerprint": TestResetFForeignFP, "flags.till": 5, "seg.s1.till": 5}, data)

	w := env.writer(t, env.client, testToken)
	require.NoError(t, w.Reset())

	meta := env.meta(t)
	assert.Equal(t, wtFingerprint, meta["fingerprint"])
	assert.Equal(t, strconv.FormatInt(testToken, 10), meta["fence"])
	assert.ElementsMatch(t, []string{"fence", "fingerprint"}, env.metaFields(meta))
	assert.Zero(t, env.existing(t, data...))
	assert.False(t, w.Status().EnvironmentMismatch)

	require.NoError(t, w.Start(nil))
	w.PutFlags(wtFlags("f1"), nil, 1)
	ftEventually(t, "write after reset", 10*time.Second, func() bool {
		s := w.Status()
		return s.Healthy && s.PendingChanges == 0 && s.LastSuccessUnixMs > 0
	})
	s := w.Status()
	assert.False(t, s.EnvironmentMismatch)
	assert.Empty(t, s.LastError)
	assert.Equal(t, int64(1), env.existing(t, env.keys.FlagData()))
}

// A boot read after a fenced Reset over a populated foreign state finds no state, it is not corrupt
func TestBootReadAfterResetIsNotCorrupt(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetboot")
	data := []string{env.keys.FlagData(), env.keys.FlagIndex(), env.keys.SegmentActive("s1"), env.keys.SegmentTombstones("s1"), env.keys.SegmentRemovalClock("s1")}
	env.seed(t, map[string]interface{}{"fingerprint": TestResetFForeignFP, "flags.till": 5, "seg.s1.till": 5}, data)

	w := env.writer(t, env.client, testToken)
	require.NoError(t, w.Reset())

	rc, err := NewReadClient(srv.cfg("ftresetboot"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })
	r, err := NewReader(rc, env.keys, wtFingerprint)
	require.NoError(t, err)
	state, err := r.Read(context.Background(), func(SegmentState) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, BootAbsent, state.Status)
	assert.NotEqual(t, BootCorrupt, state.Status)
}

// F-1 negative: a Reset whose token differs from the fence is refused and leaves the foreign fingerprint and the data as they were
func TestResetFMismatchedTokenKeepsForeignFingerprint(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-b")
	data := []string{env.keys.FlagData(), env.keys.SegmentActive("s1"), env.keys.SegmentTombstones("s1"), env.keys.SegmentRemovalClock("s1")}
	env.seed(t, map[string]interface{}{"fingerprint": TestResetFForeignFP, "flags.till": 5, "seg.s1.till": 5}, data)
	keysBefore := env.allKeys(t)
	metaBefore := env.meta(t)

	w := env.writer(t, env.client, testToken+1)
	err := w.Reset()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFenced), "got %v", err)

	assert.ElementsMatch(t, keysBefore, env.allKeys(t))
	assert.Equal(t, metaBefore, env.meta(t))
	assert.Equal(t, TestResetFForeignFP, env.meta(t)["fingerprint"])
	assert.Equal(t, int64(len(data)), env.existing(t, data...))
}

// F-1 negative: the fence in Redis is advanced past the writer's token, so the writer is stale and Reset is refused
// without touching the foreign fingerprint, the meta fields or the data
func TestResetFStaleTokenKeepsForeignFingerprint(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-f")
	data := []string{env.keys.FlagData(), env.keys.SegmentActive("s1"), env.keys.SegmentTombstones("s1"), env.keys.SegmentRemovalClock("s1")}
	env.seed(t, map[string]interface{}{"fingerprint": TestResetFForeignFP, "flags.till": 5, "seg.s1.till": 5}, data)
	require.NoError(t, srv.admin.HSet(context.Background(), env.keys.Meta(), "fence", testToken+1).Err())
	keysBefore := env.allKeys(t)
	metaBefore := env.meta(t)

	w := env.writer(t, env.client, testToken)
	err := w.Reset()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrFenced), "got %v", err)

	assert.ElementsMatch(t, keysBefore, env.allKeys(t))
	assert.Equal(t, metaBefore, env.meta(t))
	assert.Equal(t, TestResetFForeignFP, env.meta(t)["fingerprint"])
	assert.Equal(t, strconv.FormatInt(testToken+1, 10), env.meta(t)["fence"])
	assert.Equal(t, int64(len(data)), env.existing(t, data...))
}

// F-1 negative: a normal write over a foreign fingerprint is still refused, and the foreign fingerprint stays
func TestResetFNormalWriteStillRefusedOverForeignFingerprint(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-c")
	env.seed(t, map[string]interface{}{"fingerprint": TestResetFForeignFP}, nil)

	w := env.writer(t, env.client, testToken)
	require.NoError(t, w.Start(nil))
	w.PutFlags(wtFlags("f1"), nil, 1)
	ftEventually(t, "the mismatch", 10*time.Second, func() bool { return w.Status().EnvironmentMismatch })

	assert.Equal(t, ErrEnvironmentMismatch.Error(), w.Status().LastError)
	assert.Equal(t, TestResetFForeignFP, env.meta(t)["fingerprint"])
	assert.Zero(t, env.existing(t, env.keys.FlagData()))
}

// F-2: an aborted Reset leaves only the segments not yet deleted, each with its cursor; a healthy rerun removes the rest
func TestResetFAbortedResetRerunRemovesAllSegments(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-d")
	names := env.segmentNames(400)
	fixed := env.fixedKeys()
	env.seed(t, env.segmentFields(names), append(append([]string(nil), fixed...), env.segmentData(names)...))

	spy := &TestResetFSpyClient{Client: env.client, failDel: 2}
	w := env.writer(t, spy, testToken)
	require.Error(t, w.Reset())

	order := spy.order()
	require.NotEmpty(t, order)
	firstDel := -1
	for i, mode := range order {
		if mode == "del" {
			firstDel = i
			break
		}
	}
	require.NotEqual(t, -1, firstDel, "no del call was recorded")
	assert.Equal(t, "meta", order[0], "the first ResetScript call must be the meta call")
	assert.Greater(t, firstDel, 0, "no del call may precede the meta call")

	calls := spy.calls()
	require.Len(t, calls, 2, "the second del call is the one refused")
	require.LessOrEqual(t, len(calls[0].keys), resetBatch)
	deleted := map[string]bool{}
	for _, cursor := range calls[0].cursors {
		deleted[cursor] = true
	}
	require.NotEmpty(t, deleted)

	ctx := context.Background()
	flagsGone, err := srv.admin.HExists(ctx, env.keys.Meta(), "flags.till").Result()
	require.NoError(t, err)
	rbsGone, err := srv.admin.HExists(ctx, env.keys.Meta(), "rbs.till").Result()
	require.NoError(t, err)
	assert.False(t, flagsGone, "flags.till is removed by the meta call, before any data key")
	assert.False(t, rbsGone, "rbs.till is removed by the meta call, before any data key")

	for _, name := range names {
		cursor := env.keys.SegmentCursorField(name)
		has, err := srv.admin.HExists(ctx, env.keys.Meta(), cursor).Result()
		require.NoError(t, err)
		if deleted[cursor] {
			assert.False(t, has, "deleted segment %s keeps its cursor", name)
			assert.Zero(t, env.existing(t, env.keys.SegmentKeys(name)[1:]...), "deleted segment %s keeps data", name)
			continue
		}
		assert.True(t, has, "pending segment %s lost its cursor", name)
		assert.Equal(t, int64(3), env.existing(t, env.keys.SegmentKeys(name)[1:]...), "pending segment %s lost data", name)
	}

	healthy := env.writer(t, env.client, testToken)
	require.NoError(t, healthy.Reset())

	assert.ElementsMatch(t, []string{env.keys.Lease(), env.keys.Meta()}, env.allKeys(t))
	assert.ElementsMatch(t, []string{"fence", "fingerprint"}, env.metaFields(env.meta(t)))
}

// F-2 batching: no batch exceeds resetBatch keys, every segment's keys and cursor leave in the same call, and no
// segment appears in two calls
func TestResetFBatchesKeepSegmentWithItsCursor(t *testing.T) {
	srv := ftStartRedis(t)
	env := &TestResetFEnv{}
	env.open(t, srv, "ftresetf-e")
	names := env.segmentNames(400)
	fixed := env.fixedKeys()
	env.seed(t, env.segmentFields(names), append(append([]string(nil), fixed...), env.segmentData(names)...))

	spy := &TestResetFSpyClient{Client: env.client}
	w := env.writer(t, spy, testToken)
	require.NoError(t, w.Reset())

	segOf := map[string]string{}
	for _, name := range names {
		for _, k := range env.keys.SegmentKeys(name)[1:] {
			segOf[k] = name
		}
	}
	calls := spy.calls()
	require.GreaterOrEqual(t, len(calls), 3, "1207 data keys need at least three batches")
	require.Equal(t, fixed, calls[0].keys[:len(fixed)], "the fixed keys go first")

	seen := map[string]bool{}
	for i, call := range calls {
		assert.LessOrEqual(t, len(call.keys), resetBatch, "call %d exceeds the batch size", i)
		perSeg := map[string]int{}
		for _, k := range call.keys {
			if name, ok := segOf[k]; ok {
				perSeg[name]++
			}
		}
		cursors := map[string]bool{}
		for _, c := range call.cursors {
			cursors[c] = true
		}
		for name, n := range perSeg {
			assert.Equal(t, 3, n, "call %d holds a partial segment %s", i, name)
			assert.True(t, cursors[env.keys.SegmentCursorField(name)], "call %d deletes %s without its cursor", i, name)
			assert.False(t, seen[name], "segment %s straddles two calls", name)
			seen[name] = true
		}
		assert.Len(t, cursors, len(perSeg), "call %d carries cursors of segments it does not delete", i)
	}
	assert.Len(t, seen, len(names))

	assert.ElementsMatch(t, []string{env.keys.Lease(), env.keys.Meta()}, env.allKeys(t))
	assert.ElementsMatch(t, []string{"fence", "fingerprint"}, env.metaFields(env.meta(t)))
}

func TestFailoverResetFenceStaleOwnerRejected(t *testing.T) {
	e := newScriptEnv(t)
	seedWriterState(e)
	resetData := []string{e.keys.FlagData(), e.keys.FlagIndex(), e.keys.FlagRemoved()}
	require.Nil(t, resetCall(e, resetData, testOwner, testToken, "del", e.keys.FlagCursorField()))
	assert.Equal(t, strconv.FormatInt(testToken, 10), e.dumpAll()[e.keys.Meta()].Fields[FenceField], "reset dropped the fence")

	// a new holder takes the lease at the next term; the old token must now be refused by every fenced script, reset included
	e.seedLease(testOwner, testToken+1)
	before := e.dumpAll()
	for _, call := range fencedWriteCalls(e) {
		err := callFenced(e, call, testOwner, testToken)
		require.NotNil(t, err, call.name)
		assert.True(t, ErrorHasMarker(err, FencedMarker), "%s: %s", call.name, err.Error())
		assert.Equal(t, before, e.dumpAll(), "%s wrote on stale owner", call.name)
	}
}
