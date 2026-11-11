package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/conf"
	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testNow       = int64(1000000000)
	testRetention = int64(1000)
	// testLongRetention keeps tombstones for an hour so "kept" assertions never race the real Redis clock (D23)
	testLongRetention = int64(3600000)
	testFp            = "fp-A"

	dumpScript = `
local out = {}
for _, k in ipairs(redis.call('KEYS', ARGV[1])) do
  local t = redis.call('TYPE', k).ok
  local v
  if t == 'hash' then v = redis.call('HGETALL', k)
  elseif t == 'zset' then v = redis.call('ZRANGE', k, 0, -1, 'WITHSCORES')
  elseif t == 'set' then v = redis.call('SMEMBERS', k) table.sort(v)
  else v = {t} end
  out[k] = {t = t, v = v}
end
return redis.error_reply(cjson.encode(out))
`
)

// ResetScript deletes every key matching ARGV[1] (use Keys.Pattern) and returns the number of deleted keys. Test helper
// only: it uses KEYS, which production code must not run.
const ResetScript = `local keys = redis.call('KEYS', ARGV[1]) for _, k in ipairs(keys) do redis.call('DEL', k) end return #keys`

type scriptEnv struct {
	t      *testing.T
	client *redis.PrefixedRedisClient
	prefix string
	keys   Keys
}

type dumped struct {
	Type   string
	Fields map[string]string
	Scores map[string]float64
	Set    []string
}

func newScriptClient(t *testing.T, prefix string) *redis.PrefixedRedisClient {
	t.Helper()
	client, err := commonsredis.NewRedisClient(&conf.RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Database: 0,
		Network:  "tcp",
		Prefix:   prefix,
	}, logging.NewLogger(nil))
	require.Nil(t, err)
	return client
}

func newScriptEnv(t *testing.T) *scriptEnv {
	t.Helper()
	prefix := "t-" + t.Name()
	client := newScriptClient(t, prefix)
	keys, err := NewKeys(client.Prefix(), "test-api-key", "")
	require.Nil(t, err)
	e := &scriptEnv{t: t, client: client, prefix: prefix, keys: keys}
	e.wipe()
	t.Cleanup(e.wipe)
	return e
}

func (e *scriptEnv) wipe() {
	e.t.Helper()
	require.Nil(e.t, e.client.Eval(ResetScript, nil, e.prefix+".*"))
}

func (e *scriptEnv) with(namespace string) *scriptEnv {
	e.t.Helper()
	keys, err := NewKeys(e.client.Prefix(), "test-api-key", namespace)
	require.Nil(e.t, err)
	return &scriptEnv{t: e.t, client: e.client, prefix: e.prefix, keys: keys}
}

func payload(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimPrefix(err.Error(), "ERR ")
	return msg
}

func (e *scriptEnv) dump() map[string]dumped {
	e.t.Helper()
	err := e.client.Eval(dumpScript, nil, e.prefix+".*")
	require.NotNil(e.t, err)
	raw := map[string]struct {
		T string          `json:"t"`
		V json.RawMessage `json:"v"`
	}{}
	require.Nil(e.t, json.Unmarshal([]byte(payload(err)), &raw), payload(err))
	out := map[string]dumped{}
	for k, item := range raw {
		var flat []string
		if string(item.V) != "{}" {
			require.Nil(e.t, json.Unmarshal(item.V, &flat))
		}
		d := dumped{Type: item.T}
		switch item.T {
		case "hash":
			d.Fields = map[string]string{}
			for i := 0; i+1 < len(flat); i += 2 {
				d.Fields[flat[i]] = flat[i+1]
			}
		case "zset":
			d.Scores = map[string]float64{}
			for i := 0; i+1 < len(flat); i += 2 {
				score, perr := strconv.ParseFloat(flat[i+1], 64)
				require.Nil(e.t, perr)
				d.Scores[flat[i]] = score
			}
		case "set":
			d.Set = flat
		}
		out[k] = d
	}
	return out
}

func (e *scriptEnv) names() []string {
	e.t.Helper()
	names := []string{}
	for k := range e.dump() {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ent and segKey are the writer's own entity and key types, so the tests build ARGV with the production builders
type ent = entity

type segKey = segmentKey

func entityArgs(fp string, retention int64, cursorField string, cursor int64, ents ...ent) []interface{} {
	return buildEntityArgs(fp, retention, cursorField, cursor, ents)
}

func segmentArgs(fp string, retention int64, name, cursorField string, cursor int64, adds, rems []segKey) []interface{} {
	return buildSegmentArgs(fp, retention, name, cursorField, cursor, adds, rems)
}

func TestBuiltArgsRunAgainstScripts(t *testing.T) {
	e := newScriptEnv(t)
	ents := []ent{{"a", 5, false, `{"v":1}`}, {"b", 6, true, `{"v":2}`}}
	require.Nil(t, e.client.Eval(FlagScript, e.keys.FlagKeys(), buildEntityArgs(testFp, testRetention, e.keys.FlagCursorField(), 7, ents)...))

	d := e.dump()
	assert.Equal(t, map[string]string{"a": `{"v":1}`, "b": `{"v":2}`}, d[e.keys.FlagData()].Fields)
	assert.Equal(t, map[string]float64{"a": 5, "b": 6}, d[e.keys.FlagIndex()].Scores)
	assert.Equal(t, map[string]float64{"b": updatedAt(t, d, e.keys.Meta())}, d[e.keys.FlagRemoved()].Scores)
	assert.Equal(t, "7", d[e.keys.Meta()].Fields[e.keys.FlagCursorField()])

	adds := []segKey{{"k1", 3}}
	rems := []segKey{{"k2", 4}}
	require.Nil(t, e.client.Eval(SegmentScript, e.keys.SegmentKeys("s"), buildSegmentArgs(testFp, testRetention, "s", e.keys.SegmentCursorField("s"), 9, adds, rems)...))

	d = e.dump()
	assert.Equal(t, map[string]float64{"k1": 3}, d[e.keys.SegmentActive("s")].Scores)
	assert.Equal(t, map[string]float64{"k2": 4}, d[e.keys.SegmentTombstones("s")].Scores)
	assert.Equal(t, "9", d[e.keys.Meta()].Fields[e.keys.SegmentCursorField("s")])
}

func (e *scriptEnv) flags(fp string, cursor int64, ents ...ent) error {
	return e.client.Eval(FlagScript, e.keys.FlagKeys(), entityArgs(fp, testRetention, e.keys.FlagCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) rbs(fp string, cursor int64, ents ...ent) error {
	return e.client.Eval(RuleBasedSegmentScript, e.keys.RuleBasedSegmentKeys(), entityArgs(fp, testRetention, e.keys.RuleBasedSegmentCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) segment(fp string, name string, cursor int64, adds, rems []segKey) error {
	return e.segmentWithRetention(fp, testRetention, name, cursor, adds, rems)
}

func (e *scriptEnv) flagsWithRetention(fp string, retention int64, cursor int64, ents ...ent) error {
	return e.client.Eval(FlagScript, e.keys.FlagKeys(), entityArgs(fp, retention, e.keys.FlagCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) rbsWithRetention(fp string, retention int64, cursor int64, ents ...ent) error {
	return e.client.Eval(RuleBasedSegmentScript, e.keys.RuleBasedSegmentKeys(), entityArgs(fp, retention, e.keys.RuleBasedSegmentCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) segmentWithRetention(fp string, retention int64, name string, cursor int64, adds, rems []segKey) error {
	return e.client.Eval(SegmentScript, e.keys.SegmentKeys(name), segmentArgs(fp, retention, name, e.keys.SegmentCursorField(name), cursor, adds, rems)...)
}

// updatedAt returns meta updatedAtMs, written from Redis TIME by the last applied call (D23). Tests never compare it to a Go clock.
func updatedAt(t *testing.T, d map[string]dumped, metaKey string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(d[metaKey].Fields[UpdatedAtField], 64)
	require.Nil(t, err)
	require.NotZero(t, v)
	return v
}

// advanceClock repeats an accepted call until the Redis clock has moved past base. Bounded to 2s in 10ms steps (D23), no fixed sleep.
func (e *scriptEnv) advanceClock(t *testing.T, base float64, accepted func() error) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		require.Nil(t, accepted())
		if updatedAt(t, e.dump(), e.keys.Meta()) > base {
			return
		}
		require.True(t, time.Now().Before(deadline), "redis clock did not advance past the tombstone")
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScriptFlagsUpsertAndCursor(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flags(testFp, 6, ent{"a", 5, false, `{"v":"a5"}`}, ent{"b", 6, false, `{"v":"b6"}`}))
	d := e.dump()
	assert.Equal(t, map[string]string{"a": `{"v":"a5"}`, "b": `{"v":"b6"}`}, d[fk[1]].Fields)
	assert.Equal(t, map[string]float64{"a": 5, "b": 6}, d[fk[2]].Scores)
	assert.NotContains(t, d, fk[3])
	assert.Equal(t, testFp, d[fk[0]].Fields["fingerprint"])
	assert.Equal(t, "6", d[fk[0]].Fields["flags.till"])

	require.Nil(t, e.flags(testFp, 7, ent{"a", 7, false, `{"v":"a7"}`}))
	d = e.dump()
	assert.Equal(t, `{"v":"a7"}`, d[fk[1]].Fields["a"])
	assert.Equal(t, float64(7), d[fk[2]].Scores["a"])
	assert.Equal(t, "7", d[fk[0]].Fields["flags.till"])

	require.Nil(t, e.flags(testFp, 7, ent{"a", 6, false, `{"v":"a6"}`}))
	d = e.dump()
	assert.Equal(t, `{"v":"a7"}`, d[fk[1]].Fields["a"], "a stale change does not overwrite a newer one")
	assert.Equal(t, float64(7), d[fk[2]].Scores["a"], "a stale change does not move the change number")

	require.Nil(t, e.flags(testFp, 7, ent{"a", 8, true, `{"v":"a8"}`}))
	d = e.dump()
	assert.Equal(t, float64(8), d[fk[2]].Scores["a"])
	assert.Contains(t, d[fk[3]].Scores, "a", "a removal at a newer cn writes a tombstone")

	require.Nil(t, e.rbs(testFp, 3, ent{"r", 3, false, `{"r":1}`}))
	rk := e.keys.RuleBasedSegmentKeys()
	d = e.dump()
	assert.Equal(t, `{"r":1}`, d[rk[1]].Fields["r"])
	assert.Equal(t, "3", d[rk[0]].Fields["rbs.till"])
	assert.Equal(t, "7", d[rk[0]].Fields["flags.till"])
}

func TestScriptFlagsArchivedKeptThenExpired(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flagsWithRetention(testFp, testLongRetention, 10, ent{"gone", 10, true, `{"status":"ARCHIVED"}`}, ent{"live", 9, false, `{"status":"ACTIVE"}`}))
	d := e.dump()
	assert.Equal(t, `{"status":"ARCHIVED"}`, d[fk[1]].Fields["gone"])
	assert.Equal(t, updatedAt(t, d, e.keys.Meta()), d[fk[3]].Scores["gone"], "the tombstone is scored with the Redis clock")
	assert.NotContains(t, d[fk[3]].Scores, "live")

	require.Nil(t, e.flagsWithRetention(testFp, testLongRetention, -1))
	d = e.dump()
	assert.Contains(t, d[fk[1]].Fields, "gone", "kept under a long retention")

	require.Nil(t, e.flagsWithRetention(testFp, 0, -1))
	d = e.dump()
	assert.NotContains(t, d[fk[1]].Fields, "gone", "expired on the next purge with retention 0")
	assert.NotContains(t, d[fk[2]].Scores, "gone")
	assert.Contains(t, d[fk[1]].Fields, "live")
	assert.Contains(t, d[fk[2]].Scores, "live")
	assert.NotContains(t, d, fk[3])
}

func TestScriptStaleChangeDoesNotOverwriteNewer(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flags(testFp, 10, ent{"a", 10, false, `{"v":10}`}))
	require.Nil(t, e.flags(testFp, 10, ent{"a", 5, false, `{"v":5}`}))
	d := e.dump()
	assert.Equal(t, `{"v":10}`, d[fk[1]].Fields["a"])
	assert.Equal(t, float64(10), d[fk[2]].Scores["a"])

	require.Nil(t, e.flags(testFp, 10, ent{"a", 5, true, `{"v":"removed5"}`}))
	d = e.dump()
	assert.Equal(t, `{"v":10}`, d[fk[1]].Fields["a"])
	assert.NotContains(t, d, fk[3])

	require.Nil(t, e.flags(testFp, 11, ent{"a", 11, true, `{"v":"removed11"}`}))
	require.Nil(t, e.flags(testFp, 11, ent{"a", 10, false, `{"v":10}`}))
	d = e.dump()
	assert.Equal(t, `{"v":"removed11"}`, d[fk[1]].Fields["a"])
	assert.Contains(t, d[fk[3]].Scores, "a")
}

func TestScriptSegmentAddRemoveTombstone(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")

	require.Nil(t, e.segment(testFp, "seg.one", 1, []segKey{{"k1", 1}, {"k2", 1}}, nil))
	d := e.dump()
	assert.Equal(t, map[string]float64{"k1": 1, "k2": 1}, d[sk[1]].Scores)
	assert.Equal(t, "1", d[sk[0]].Fields["seg.seg.one.till"])

	require.Nil(t, e.segment(testFp, "seg.one", 2, nil, []segKey{{"k1", 2}}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k2": 1}, d[sk[1]].Scores)
	assert.Equal(t, map[string]float64{"k1": 2}, d[sk[2]].Scores)
	assert.Equal(t, map[string]float64{"k1": updatedAt(t, d, sk[0])}, d[sk[3]].Scores)
	assert.Equal(t, "2", d[sk[0]].Fields["seg.seg.one.till"])

	require.Nil(t, e.segment(testFp, "seg.one", 3, []segKey{{"k1", 3}}, nil))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k1": 3, "k2": 1}, d[sk[1]].Scores)
	assert.NotContains(t, d, sk[2])
	assert.NotContains(t, d, sk[3])

	require.Nil(t, e.segment(testFp, "seg.one", 5, nil, []segKey{{"k2", 5}}))
	require.Nil(t, e.segment(testFp, "seg.one", 5, []segKey{{"k2", 4}}, nil))
	d = e.dump()
	assert.NotContains(t, d[sk[1]].Scores, "k2")
	assert.Equal(t, float64(5), d[sk[2]].Scores["k2"])
}

func TestSegmentTieRemovalWins(t *testing.T) {
	tests := []struct {
		name  string
		calls [][2][]segKey
	}{
		{"add then remove in separate calls", [][2][]segKey{{{{"k", 5}}, nil}, {nil, {{"k", 5}}}}},
		{"remove then add in separate calls", [][2][]segKey{{nil, {{"k", 5}}}, {{{"k", 5}}, nil}}},
		{"add and remove in one call", [][2][]segKey{{{{"k", 5}}, {{"k", 5}}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newScriptEnv(t)
			sk := e.keys.SegmentKeys("s")
			for i, c := range tc.calls {
				require.Nil(t, e.segment(testFp, "s", int64(i+1), c[0], c[1]))
			}
			d := e.dump()
			assert.NotContains(t, d[sk[1]].Scores, "k")
			assert.Equal(t, float64(5), d[sk[2]].Scores["k"])
		})
	}
}

func TestScriptSegmentTombstoneExpires(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("s")

	require.Nil(t, e.segmentWithRetention(testFp, testLongRetention, "s", 2, []segKey{{"keep", 1}}, []segKey{{"old", 2}}))
	d := e.dump()
	assert.Contains(t, d[sk[2]].Scores, "old")

	require.Nil(t, e.segmentWithRetention(testFp, testLongRetention, "s", -1, nil, nil))
	assert.Contains(t, e.dump()[sk[2]].Scores, "old", "kept under a long retention")

	require.Nil(t, e.segmentWithRetention(testFp, 0, "s", -1, nil, nil))
	d = e.dump()
	assert.NotContains(t, d, sk[2])
	assert.NotContains(t, d, sk[3])
	assert.Contains(t, d[sk[1]].Scores, "keep")
}

func TestScriptEnvironmentMismatchWritesNothing(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(testFp, 5, ent{"a", 5, false, `{"v":1}`}))
	require.Nil(t, e.segment(testFp, "s", 1, []segKey{{"k", 1}}, nil))
	before := e.dump()

	err := e.flags("fp-B", 9, ent{"a", 9, false, `{"v":2}`}, ent{"new", 9, false, `{}`})
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	err = e.rbs("fp-B", 9, ent{"r", 9, false, `{}`})
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	err = e.segment("fp-B", "s", 9, []segKey{{"x", 9}}, nil)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	assert.Equal(t, before, e.dump())
}

func TestScriptUpdatedAtSetOnApplyNotOnRefusal(t *testing.T) {
	e := newScriptEnv(t)
	meta := func() map[string]string { return e.dump()[e.keys.Meta()].Fields }

	require.Nil(t, e.flags(testFp, 5, ent{"a", 5, false, `{"v":1}`}))
	applied := updatedAt(t, e.dump(), e.keys.Meta())
	e.advanceClock(t, applied, func() error { return e.flags(testFp, 5, ent{"a", 5, false, `{"v":1}`}) })
	mark := meta()[UpdatedAtField]

	err := e.flags("fp-B", 9, ent{"a", 9, false, `{"v":2}`})
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())
	assert.Equal(t, mark, meta()[UpdatedAtField], "a refused call must not move updatedAtMs")
}

func TestSegmentScriptUpdatedAtNotSetOnRefusal(t *testing.T) {
	e := newScriptEnv(t)
	meta := func() map[string]string { return e.dump()[e.keys.Meta()].Fields }

	require.Nil(t, e.segment(testFp, "s", 1, []segKey{{"k", 1}}, nil))
	applied := updatedAt(t, e.dump(), e.keys.Meta())
	e.advanceClock(t, applied, func() error { return e.segment(testFp, "s", 1, []segKey{{"k", 1}}, nil) })
	mark := meta()[UpdatedAtField]

	before := e.dump()
	err := e.segment("fp-B", "s", 2, []segKey{{"k", 2}}, nil)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())
	assert.Equal(t, mark, meta()[UpdatedAtField], "a refused call must not move updatedAtMs")
	assert.Equal(t, before, e.dump())
}

func TestScriptBadArgsWritesNothing(t *testing.T) {
	e := newScriptEnv(t)

	// Seed valid state so a partial write would be visible as a change
	require.Nil(t, e.flags(testFp, 1, ent{"seed", 1, false, `{}`}))
	require.Nil(t, e.segment(testFp, "s", 1, []segKey{{"seedk", 1}}, nil))
	before := e.dump()

	// ARGV layout: 1 fp, 2 retention, 3 cursorField, 4 cursor, 5 n, then per entity name, cn, removed, json
	args := entityArgs(testFp, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	err := e.client.Eval(FlagScript, e.keys.FlagKeys(), args[:len(args)-1]...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	args = entityArgs(testFp, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[4] = "x"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	// Non-numeric change number and invalid removed flag, both must be refused before any write
	args = entityArgs(testFp, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[6] = "nope"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	args = entityArgs(testFp, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[7] = "2"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	// Segment ARGV: 1 fp, 2 retention, 3 name, 4 cursorField, 5 cursor, 6 nAdd, 7 nRem, then (key, cn) pairs
	sargs := segmentArgs(testFp, testRetention, "s", e.keys.SegmentCursorField("s"), 1, []segKey{{"k", 1}}, nil)
	err = e.client.Eval(SegmentScript, e.keys.SegmentKeys("s"), append(sargs, "extra")...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	sargs = segmentArgs(testFp, testRetention, "s", e.keys.SegmentCursorField("s"), 1, []segKey{{"k", 1}}, nil)
	sargs[8] = "nope"
	err = e.client.Eval(SegmentScript, e.keys.SegmentKeys("s"), sargs...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	assert.Equal(t, before, e.dump(), "refused calls must not change any stored key")
}

func TestScriptCursorOnlyMovesForward(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()
	sk := e.keys.SegmentKeys("s")

	for _, step := range []struct{ send, want string }{{"10", "10"}, {"5", "10"}, {"-1", "10"}, {"10", "10"}, {"11", "11"}} {
		cursor, err := strconv.ParseInt(step.send, 10, 64)
		require.Nil(t, err)
		require.Nil(t, e.flags(testFp, cursor))
		require.Nil(t, e.segment(testFp, "s", cursor, nil, nil))
		d := e.dump()
		assert.Equal(t, step.want, d[fk[0]].Fields["flags.till"], "send %s", step.send)
		assert.Equal(t, step.want, d[sk[0]].Fields["seg.s.till"], "send %s", step.send)
	}
}

func TestScriptCreatesOnlyDocumentedKeys(t *testing.T) {
	e := newScriptEnv(t)

	require.Nil(t, e.flags(testFp, 3, ent{"f1", 2, false, `{}`}, ent{"f2", 3, true, `{}`}))
	require.Nil(t, e.rbs(testFp, 3, ent{"r1", 2, false, `{}`}, ent{"r2", 3, true, `{}`}))
	require.Nil(t, e.segment(testFp, "seg.x", 3, []segKey{{"k1", 1}, {"k2", 1}}, []segKey{{"k2", 3}}))

	want := map[string]bool{}
	for _, list := range [][]string{e.keys.FlagKeys(), e.keys.RuleBasedSegmentKeys(), e.keys.SegmentKeys("seg.x")} {
		for _, k := range list {
			want[k] = true
		}
	}
	wantList := []string{}
	for k := range want {
		wantList = append(wantList, k)
	}
	sort.Strings(wantList)
	assert.Equal(t, wantList, e.names())
}

func TestScriptTombstoneClockNotRefreshedByResend(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("s")
	fk := e.keys.FlagKeys()

	require.Nil(t, e.segment(testFp, "s", 2, nil, []segKey{{"k", 2}}))
	d := e.dump()
	tomb := updatedAt(t, d, sk[0])
	assert.Equal(t, map[string]float64{"k": tomb}, d[sk[3]].Scores, fmt.Sprint(d[sk[3]]))

	e.advanceClock(t, tomb, func() error { return e.segment(testFp, "s", 2, nil, []segKey{{"k", 2}}) })
	assert.Equal(t, map[string]float64{"k": tomb}, e.dump()[sk[3]].Scores, "resend must not refresh the segment tombstone clock")

	require.Nil(t, e.flags(testFp, 2, ent{"f", 2, true, `{}`}))
	d = e.dump()
	flagTomb := updatedAt(t, d, fk[0])
	assert.Equal(t, map[string]float64{"f": flagTomb}, d[fk[3]].Scores)

	e.advanceClock(t, flagTomb, func() error { return e.flags(testFp, 2, ent{"f", 2, true, `{}`}) })
	assert.Equal(t, map[string]float64{"f": flagTomb}, e.dump()[fk[3]].Scores, "resend must not refresh the flag tombstone clock")
}

func TestResetOnlyDeletesOwnNamespace(t *testing.T) {
	a := newScriptEnv(t)
	b := a.with("other-env")

	require.Nil(t, a.flags(testFp, 1, ent{"f", 1, false, `{}`}))
	require.Nil(t, a.segment(testFp, "s", 1, []segKey{{"k", 1}}, nil))
	require.Nil(t, b.flags(testFp, 1, ent{"f", 1, false, `{}`}))
	require.Nil(t, a.client.Set("SPLITIO.split.x", "value", 0))

	require.Nil(t, a.client.Eval(ResetScript, nil, a.keys.Pattern()))

	for _, name := range a.names() {
		assert.NotContains(t, name, "{SPLITIO_PROXY."+a.keys.Env()+"}", name)
	}
	assert.Contains(t, a.names(), b.keys.FlagKeys()[1])
	assert.Contains(t, a.names(), a.prefix+".SPLITIO.split.x")
	assert.Len(t, a.names(), 4)
}

func TestWriterCrashBetweenSegmentChunks(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")

	// D22: the first chunk registers the name with cursor -1, so a crash here leaves a name that Reset and purge can see
	require.Nil(t, e.segment(testFp, "seg.one", noCursor, []segKey{{"k1", 1}}, nil))
	d := e.dump()
	assert.Equal(t, map[string]float64{"k1": 1}, d[sk[1]].Scores)
	assert.Equal(t, "-1", d[sk[0]].Fields["seg.seg.one.till"], "partially written segment keeps cursor -1")
	assert.True(t, ErrorHasMarker(e.client.Eval(SegmentCheckScript, sk, 1, "seg.one"), countMismatchMarker), "a partially written segment is treated as missing")

	require.Nil(t, e.segment(testFp, "seg.one", 1, []segKey{{"k2", 1}}, nil))
	d = e.dump()
	assert.Equal(t, "1", d[sk[0]].Fields["seg.seg.one.till"])
}

// segEval is one SegmentScript call as the writer sent it: the cursor ARGV and the adds and removes it carried
type segEval struct {
	cursor int64
	adds   []string
	rems   []string
}

// segmentCallRecorder records the SegmentScript calls made through the Writer and can fail them from a given call on.
// A failing call is not forwarded to Redis, which simulates a crash or a chunk that never lands.
type segmentCallRecorder struct {
	Client
	failFrom int
	calls    []segEval
	seen     int
}

func (c *segmentCallRecorder) Eval(script string, keys []string, args ...interface{}) error {
	if script != SegmentScript {
		return c.Client.Eval(script, keys, args...)
	}
	c.seen++
	if c.failFrom > 0 && c.seen >= c.failFrom {
		return errors.New("simulated crash between segment chunks")
	}
	// buildSegmentArgs layout: fingerprint, retention, name, cursorField, cursor, nAdds, nRems, then key,cn pairs
	nAdds, nRems := args[5].(int), args[6].(int)
	call := segEval{cursor: args[4].(int64)}
	for i := 0; i < nAdds+nRems; i++ {
		key := args[7+2*i].(string)
		if i < nAdds {
			call.adds = append(call.adds, key)
		} else {
			call.rems = append(call.rems, key)
		}
	}
	c.calls = append(c.calls, call)
	return c.Client.Eval(script, keys, args...)
}

func TestResetAfterPartialSegmentWriteLeavesNoKeys(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")
	field := e.keys.SegmentCursorField("seg.one")

	// two chunks of one key each; chunk 2 never lands, so the write stops after chunk 1 (D22, crash point FL-3)
	rc := &segmentCallRecorder{Client: e.client, failFrom: 2}
	opts := DefaultOptions()
	opts.ChunkSize = 1
	opts.MaxAttempts = 1
	w, err := NewWriter(rc, e.keys, testFp, opts, logging.NewLogger(nil))
	require.Nil(t, err)
	_, err = w.writeSegment("seg.one", []segKey{{"k1", 1}, {"k2", 1}}, nil, 1, false)
	require.Nil(t, err)
	require.Len(t, rc.calls, 1, "exactly chunk 1 reached Redis")
	assert.Equal(t, noCursor, rc.calls[0].cursor, "chunk 1 carries no real cursor")

	d := e.dump()
	assert.Equal(t, "-1", d[sk[0]].Fields[field], "chunk 1 registered the name with cursor -1")
	assert.Equal(t, map[string]float64{"k1": 1}, d[sk[1]].Scores, "chunk 1 data is written and chunk 2 data is not")

	w2, err := NewWriter(e.client, e.keys, testFp, DefaultOptions(), logging.NewLogger(nil))
	require.Nil(t, err)
	require.Nil(t, w2.Reset())

	names := e.names()
	for _, k := range sk {
		assert.NotContains(t, names, k, "Reset removes every key of the partially written segment")
	}
}

func TestScriptSegmentCheckDetectsMissingKeys(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")

	err := e.client.Eval(SegmentCheckScript, sk, 0, "seg.one")
	assert.True(t, ErrorHasMarker(err, countMismatchMarker), "unknown segment")

	require.Nil(t, e.segment(testFp, "seg.one", 1, []segKey{{"k1", 1}, {"k2", 1}}, nil))
	assert.Nil(t, e.client.Eval(SegmentCheckScript, sk, 2, "seg.one"))
	err = e.client.Eval(SegmentCheckScript, sk, 3, "seg.one")
	assert.True(t, ErrorHasMarker(err, countMismatchMarker), "count differs")
}

func TestScriptRuleBasedSegmentArchivedKeptThenExpired(t *testing.T) {
	e := newScriptEnv(t)
	rk := e.keys.RuleBasedSegmentKeys()

	require.Nil(t, e.rbsWithRetention(testFp, testLongRetention, 10, ent{"gone", 10, true, `{"status":"ARCHIVED"}`}, ent{"live", 9, false, `{"status":"ACTIVE"}`}))
	d := e.dump()
	assert.Equal(t, `{"status":"ARCHIVED"}`, d[rk[1]].Fields["gone"])
	assert.Equal(t, updatedAt(t, d, e.keys.Meta()), d[rk[3]].Scores["gone"], "the tombstone is scored with the Redis clock")
	assert.NotContains(t, d[rk[3]].Scores, "live")

	require.Nil(t, e.rbsWithRetention(testFp, testLongRetention, -1))
	d = e.dump()
	assert.Contains(t, d[rk[1]].Fields, "gone", "kept under a long retention")
	assert.Contains(t, d[rk[2]].Scores, "gone")
	assert.Contains(t, d[rk[3]].Scores, "gone")

	require.Nil(t, e.rbsWithRetention(testFp, 0, -1))
	d = e.dump()
	assert.NotContains(t, d[rk[1]].Fields, "gone", "expired on the next purge with retention 0")
	assert.NotContains(t, d[rk[2]].Scores, "gone")
	assert.NotContains(t, d, rk[3])
	assert.Contains(t, d[rk[1]].Fields, "live")
	assert.Contains(t, d[rk[2]].Scores, "live")
}

func TestSegmentCheckUsesCursorField(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")
	field := e.keys.SegmentCursorField("seg.one")
	check := func(count int64) error { return e.client.Eval(SegmentCheckScript, sk, count, "seg.one") }

	require.Nil(t, e.client.Eval(`redis.call('ZADD', KEYS[1], 1, 'k1') return 1`, sk[1:2]))
	assert.True(t, ErrorHasMarker(check(1), countMismatchMarker), "cursor field missing")

	require.Nil(t, e.client.Eval(`redis.call('HSET', KEYS[1], ARGV[1], '-1') return 1`, sk[0:1], field))
	assert.True(t, ErrorHasMarker(check(1), countMismatchMarker), "cursor field -1 is partial")

	require.Nil(t, e.client.Eval(`redis.call('HSET', KEYS[1], ARGV[1], '3') return 1`, sk[0:1], field))
	assert.Nil(t, check(1), "cursor >= 0 and matching count is intact")
	assert.True(t, ErrorHasMarker(check(2), countMismatchMarker), "count differs")

	require.Nil(t, e.segment(testFp, "seg.empty", 4, nil, nil))
	assert.Nil(t, e.client.Eval(SegmentCheckScript, e.keys.SegmentKeys("seg.empty"), 0, "seg.empty"), "empty segment with cursor >= 0 is intact")

	for _, n := range e.names() {
		assert.False(t, strings.HasSuffix(n, ".segments"), "no .segments key: %s", n)
	}
}

func TestScriptMetaWrongTypeIsTypedDataWrongTypeIsPlain(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()
	cf := e.keys.FlagCursorField()
	args := entityArgs(testFp, testRetention, cf, 6, ent{"a", 5, false, `{}`})
	require.Nil(t, e.client.Eval(`return redis.call('SET', KEYS[1], 'x')`, fk[0:1]))
	err := e.client.Eval(FlagScript, fk, args...)
	assert.True(t, ErrorHasMarker(err, MetaWrongTypeMarker), "meta WRONGTYPE must return the typed marker, got %v", err)
	require.Nil(t, e.client.Eval(`return redis.call('DEL', KEYS[1])`, fk[0:1]))
	require.Nil(t, e.client.Eval(`return redis.call('SET', KEYS[1], 'x')`, fk[1:2]))
	err = e.client.Eval(FlagScript, fk, args...)
	assert.True(t, ErrorHasMarker(err, wrongTypeMarker), "entity WRONGTYPE stays plain, got %v", err)
	assert.False(t, ErrorHasMarker(err, MetaWrongTypeMarker), "entity WRONGTYPE must not carry the meta marker")
}

func TestScriptRejectsInvalidNumbers(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()
	cf := e.keys.FlagCursorField()
	rejected := func(err error) {
		t.Helper()
		assert.True(t, ErrorHasMarker(err, BadArgsMarker), "expected PROXY_BAD_ARGS, got %v", err)
		assert.Empty(t, e.names(), "a rejected call writes nothing")
	}

	rejected(e.client.Eval(FlagScript, fk, entityArgs(testFp, -1, cf, 6, ent{"a", 5, false, `{}`})...))

	cursorText := entityArgs(testFp, testRetention, cf, 6, ent{"a", 5, false, `{}`})
	cursorText[3] = "1.5"
	rejected(e.client.Eval(FlagScript, fk, cursorText...))

	countText := entityArgs(testFp, testRetention, cf, 6, ent{"a", 5, false, `{}`})
	countText[4] = "1e3"
	rejected(e.client.Eval(FlagScript, fk, countText...))

	entityCn := entityArgs(testFp, testRetention, cf, 6, ent{"a", 5, false, `{}`})
	entityCn[6] = "1.5"
	rejected(e.client.Eval(FlagScript, fk, entityCn...))

	sk := e.keys.SegmentKeys("seg.one")
	segRetention := segmentArgs(testFp, -1, "seg.one", e.keys.SegmentCursorField("seg.one"), 1, []segKey{{"k1", 1}}, nil)
	rejected(e.client.Eval(SegmentScript, sk, segRetention...))

	segCount := segmentArgs(testFp, testRetention, "seg.one", e.keys.SegmentCursorField("seg.one"), 1, []segKey{{"k1", 1}}, nil)
	segCount[5] = "1.5"
	rejected(e.client.Eval(SegmentScript, sk, segCount...))
}

func TestSegmentRegisteredOnFirstChunk(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")
	field := e.keys.SegmentCursorField("seg.one")

	require.Nil(t, e.segment(testFp, "seg.one", noCursor, []segKey{{"k1", 1}}, nil))
	assert.Equal(t, "-1", e.dump()[sk[0]].Fields[field], "first chunk registers the name with cursor -1")

	require.Nil(t, e.segment(testFp, "seg.one", 10, []segKey{{"k2", 10}}, nil))
	assert.Equal(t, "10", e.dump()[sk[0]].Fields[field], "last chunk sets the cursor")

	require.Nil(t, e.segment(testFp, "seg.one", noCursor, []segKey{{"k3", 10}}, nil))
	assert.Equal(t, "10", e.dump()[sk[0]].Fields[field], "a later first chunk never lowers the cursor")
}

func TestSegmentTieAcrossChunks(t *testing.T) {
	e := newScriptEnv(t)

	addThenRemove := e.keys.SegmentKeys("seg.add")
	require.Nil(t, e.segment(testFp, "seg.add", noCursor, []segKey{{"k1", 5}}, nil))
	require.Nil(t, e.segment(testFp, "seg.add", 5, nil, []segKey{{"k1", 5}}))
	d := e.dump()
	assert.Empty(t, d[addThenRemove[1]].Scores, "removal wins on an equal cn (D37)")
	assert.Equal(t, map[string]float64{"k1": 5}, d[addThenRemove[2]].Scores)

	removeThenAdd := e.keys.SegmentKeys("seg.rem")
	require.Nil(t, e.segment(testFp, "seg.rem", noCursor, nil, []segKey{{"k2", 5}}))
	require.Nil(t, e.segment(testFp, "seg.rem", 5, []segKey{{"k2", 5}}, nil))
	d = e.dump()
	assert.Empty(t, d[removeThenAdd[1]].Scores, "removal wins in the reverse order too (D37)")
	assert.Equal(t, map[string]float64{"k2": 5}, d[removeThenAdd[2]].Scores)

	lower := e.keys.SegmentKeys("seg.low")
	require.Nil(t, e.segment(testFp, "seg.low", noCursor, []segKey{{"k3", 9}}, nil))
	require.Nil(t, e.segment(testFp, "seg.low", 9, nil, []segKey{{"k3", 4}}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k3": 9}, d[lower[1]].Scores, "a lower cn arriving later is ignored")
	assert.Empty(t, d[lower[2]].Scores)

	// one segment in three chunks: the chunks go out in order and only the last one carries the real cursor (D1)
	rc := &segmentCallRecorder{Client: e.client}
	opts := DefaultOptions()
	opts.ChunkSize = 1
	w, err := NewWriter(rc, e.keys, testFp, opts, logging.NewLogger(nil))
	require.Nil(t, err)
	_, err = w.writeSegment("seg.chunk", []segKey{{"k4", 5}, {"k5", 5}}, []segKey{{"k4", 5}}, 5, false)
	require.Nil(t, err)
	require.Len(t, rc.calls, 3, "one Eval per chunk")
	assert.Equal(t, segEval{cursor: noCursor, adds: []string{"k4"}}, rc.calls[0], "chunk 1 is the first add, no real cursor")
	assert.Equal(t, segEval{cursor: noCursor, adds: []string{"k5"}}, rc.calls[1], "chunk 2 is the second add, no real cursor")
	assert.Equal(t, segEval{cursor: 5, rems: []string{"k4"}}, rc.calls[2], "the last chunk is the remove and carries the cursor")
	chunk := e.keys.SegmentKeys("seg.chunk")
	d = e.dump()
	assert.Equal(t, map[string]float64{"k5": 5}, d[chunk[1]].Scores, "removal wins on an equal cn across chunks (D37)")
	assert.Equal(t, map[string]float64{"k4": 5}, d[chunk[2]].Scores)
}

func TestRetentionUsesRedisTime(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flagsWithRetention(testFp, testLongRetention, 6, ent{"a", 5, true, `{"a":1}`}))
	assert.Contains(t, e.dump()[fk[3]].Scores, "a", "tombstone kept under a long window")
	require.Nil(t, e.flagsWithRetention(testFp, 0, 6))
	assert.NotContains(t, e.dump()[fk[3]].Scores, "a", "retentionMs=0 purges the earlier tombstone on the next call")

	require.Nil(t, e.flagsWithRetention(testFp, testLongRetention, 7, ent{"b", 6, true, `{"b":1}`}))
	deadline := time.Now().Add(2 * time.Second)
	for {
		require.Nil(t, e.flagsWithRetention(testFp, 1, 7))
		if _, ok := e.dump()[fk[3]].Scores["b"]; !ok {
			break
		}
		require.True(t, time.Now().Before(deadline), "tombstone b not purged within 2s of Redis time")
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPartialRewriteKeepsCursorButFailsCheck(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")
	field := e.keys.SegmentCursorField("seg.one")

	require.Nil(t, e.segment(testFp, "seg.one", 100, []segKey{{"k1", 1}, {"k2", 1}}, nil))
	require.Nil(t, e.segment(testFp, "seg.one", noCursor, []segKey{{"k3", 2}}, nil))
	assert.Equal(t, "100", e.dump()[sk[0]].Fields[field], "a partial rewrite keeps the old cursor")
	assert.True(t, ErrorHasMarker(e.client.Eval(SegmentCheckScript, sk, 4, "seg.one"), countMismatchMarker), "the full count of 4 fails against 3 keys")
}

func TestFrozenNewSegmentCursorMinusOneCheck(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.new")

	require.Nil(t, e.segment(testFp, "seg.new", noCursor, []segKey{{"k1", 1}}, nil))
	for i := 0; i < 2; i++ {
		assert.True(t, ErrorHasMarker(e.client.Eval(SegmentCheckScript, sk, 1, "seg.new"), countMismatchMarker), "startup check %d asks for a rewrite", i)
	}

	require.Nil(t, e.segment(testFp, "seg.new", 3, []segKey{{"k1", 1}}, nil))
	assert.Nil(t, e.client.Eval(SegmentCheckScript, sk, 1, "seg.new"), "a clean write sets the cursor and the check passes")
}

func TestPurgeRefusesForeignFingerprint(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(testFp, 5, ent{"a", 5, false, `{"v":1}`}, ent{"gone", 6, true, `{}`}))
	before := e.dump()

	err := e.client.Eval(PurgeScript, e.keys.FlagKeys(), "fp-B", int64(0), int64(500), int64(1))
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())
	assert.Equal(t, before, e.dump(), "a foreign fingerprint deletes nothing, even an expired removal")

	w, werr := NewWriter(e.client, e.keys, "fp-B", wtOptions(), &wtLogger{})
	require.NoError(t, werr)
	assert.ErrorIs(t, w.purge(map[string]string{"seg.s.till": "1"}), ErrEnvironmentMismatch)
	assert.True(t, w.Status().EnvironmentMismatch)
	assert.Equal(t, before, e.dump())
}

func TestPurgeRunsWithoutSource(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(testFp, 5, ent{"a", 5, false, `{"v":1}`}, ent{"gone", 6, true, `{}`}))
	time.Sleep(5 * time.Millisecond)

	opts := wtOptions()
	opts.Retention = time.Millisecond
	w, err := NewWriter(e.client, e.keys, testFp, opts, &wtLogger{})
	require.NoError(t, err)
	require.NoError(t, w.purge(map[string]string{}))

	fk := e.keys.FlagKeys()
	d := e.dump()
	assert.Contains(t, d[fk[1]].Fields, "a", "a live flag is kept")
	assert.NotContains(t, d[fk[1]].Fields, "gone")
	assert.NotContains(t, d[fk[2]].Scores, "gone")
	assert.NotContains(t, d[fk[3]].Scores, "gone")
}

// boundarySeedScript writes one removal marker: KEYS[1] tombstones (score 1), KEYS[2] removal clock (ARGV[2]).
const boundarySeedScript = `redis.call('ZADD', KEYS[1], 1, ARGV[1])
redis.call('ZADD', KEYS[2], ARGV[2], ARGV[1])
return 1
`

// boundaryClockScript stores the Redis clock in the meta hash field probe, so the test can read it back.
const boundaryClockScript = "\n" + luaPrelude + `redis.call('HSET', KEYS[1], 'probe', nowMs())
return 1
`

func boundaryClock(t *testing.T, e *scriptEnv, meta string) int64 {
	t.Helper()
	require.Nil(t, e.client.Eval(boundaryClockScript, []string{meta}))
	v, err := strconv.ParseInt(e.dump()[meta].Fields["probe"], 10, 64)
	require.Nil(t, err)
	return v
}

// F23: PurgeScript expires a removal marker when its clock score is at or before now - retention (inclusive).
// The markers sit exactly at the cutoff, one below it (expired) and one above it (kept, the retention+1 case).
// The Redis clock is sampled before and after the purge; the attempt is repeated if the millisecond ticked, so the
// cutoff the script uses is known exactly.
func TestScriptPurgeRetentionBoundary(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("s")
	meta := e.keys.Meta()
	const retention = int64(1000)
	for attempt := 0; ; attempt++ {
		require.Less(t, attempt, 50, "the Redis clock ticked between the samples on every attempt")
		e.wipe()
		t0 := boundaryClock(t, e, meta)
		cutoff := t0 - retention
		require.Nil(t, e.client.Eval(boundarySeedScript, []string{sk[2], sk[3]}, "at", cutoff))
		require.Nil(t, e.client.Eval(boundarySeedScript, []string{sk[2], sk[3]}, "below", cutoff-1))
		require.Nil(t, e.client.Eval(boundarySeedScript, []string{sk[2], sk[3]}, "above", cutoff+1))
		require.Nil(t, e.client.Eval(PurgeScript, append(slices.Clone(sk), e.keys.PurgeLast()), testFp, retention, int64(100), int64(0)))
		if boundaryClock(t, e, meta) != t0 {
			continue
		}
		d := e.dump()
		assert.Equal(t, map[string]float64{"above": float64(cutoff + 1)}, d[sk[3]].Scores, "retention+1 is kept")
		assert.Equal(t, map[string]float64{"above": 1}, d[sk[2]].Scores, "the markers at and below the cutoff left the index")
		count, err := e.client.Get(e.keys.Unprefixed().PurgeLast())
		require.NoError(t, err)
		assert.Equal(t, "2", count, "the at-cutoff and below-cutoff markers were expired, counted in the purge count key")
		assert.NotContains(t, d[meta].Fields, "purgeLast", "the count no longer grows the meta hash")
		return
	}
}
