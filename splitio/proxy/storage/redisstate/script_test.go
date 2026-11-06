package redisstate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

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
	testFp        = "fp-A"

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

type ent struct {
	name    string
	cn      int64
	removed bool
	json    string
}

type segKey struct {
	key string
	cn  int64
}

func entityArgs(fp string, now, retention int64, cursorField string, cursor int64, ents ...ent) []interface{} {
	args := []interface{}{fp, now, retention, cursorField, cursor, len(ents)}
	for _, en := range ents {
		removed := "0"
		if en.removed {
			removed = "1"
		}
		args = append(args, en.name, en.cn, removed, en.json)
	}
	return args
}

func segmentArgs(fp string, now, retention int64, name, cursorField string, cursor int64, adds, rems []segKey) []interface{} {
	args := []interface{}{fp, now, retention, name, cursorField, cursor, len(adds), len(rems)}
	for _, a := range adds {
		args = append(args, a.key, a.cn)
	}
	for _, r := range rems {
		args = append(args, r.key, r.cn)
	}
	return args
}

func (e *scriptEnv) flags(fp string, now int64, cursor int64, ents ...ent) error {
	return e.client.Eval(FlagScript, e.keys.FlagKeys(), entityArgs(fp, now, testRetention, e.keys.FlagCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) rbs(fp string, now int64, cursor int64, ents ...ent) error {
	return e.client.Eval(RuleBasedSegmentScript, e.keys.RuleBasedSegmentKeys(), entityArgs(fp, now, testRetention, e.keys.RuleBasedSegmentCursorField(), cursor, ents...)...)
}

func (e *scriptEnv) segment(fp string, now int64, name string, cursor int64, adds, rems []segKey) error {
	return e.client.Eval(SegmentScript, e.keys.SegmentKeys(name), segmentArgs(fp, now, testRetention, name, e.keys.SegmentCursorField(name), cursor, adds, rems)...)
}

func TestScriptFlagsUpsertAndCursor(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flags(testFp, testNow, 6, ent{"a", 5, false, `{"v":"a5"}`}, ent{"b", 6, false, `{"v":"b6"}`}))
	d := e.dump()
	assert.Equal(t, map[string]string{"a": `{"v":"a5"}`, "b": `{"v":"b6"}`}, d[fk[1]].Fields)
	assert.Equal(t, map[string]float64{"a": 5, "b": 6}, d[fk[2]].Scores)
	assert.NotContains(t, d, fk[3])
	assert.Equal(t, testFp, d[fk[0]].Fields["fingerprint"])
	assert.Equal(t, "6", d[fk[0]].Fields["flags.till"])

	require.Nil(t, e.flags(testFp, testNow, 7, ent{"a", 7, false, `{"v":"a7"}`}))
	d = e.dump()
	assert.Equal(t, `{"v":"a7"}`, d[fk[1]].Fields["a"])
	assert.Equal(t, float64(7), d[fk[2]].Scores["a"])
	assert.Equal(t, "7", d[fk[0]].Fields["flags.till"])

	require.Nil(t, e.rbs(testFp, testNow, 3, ent{"r", 3, false, `{"r":1}`}))
	rk := e.keys.RuleBasedSegmentKeys()
	d = e.dump()
	assert.Equal(t, `{"r":1}`, d[rk[1]].Fields["r"])
	assert.Equal(t, "3", d[rk[0]].Fields["rbs.till"])
	assert.Equal(t, "7", d[rk[0]].Fields["flags.till"])
}

func TestScriptFlagsArchivedKeptThenExpired(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flags(testFp, testNow, 10, ent{"gone", 10, true, `{"status":"ARCHIVED"}`}, ent{"live", 9, false, `{"status":"ACTIVE"}`}))
	d := e.dump()
	assert.Equal(t, `{"status":"ARCHIVED"}`, d[fk[1]].Fields["gone"])
	assert.Equal(t, float64(testNow), d[fk[3]].Scores["gone"])
	assert.NotContains(t, d[fk[3]].Scores, "live")

	require.Nil(t, e.flags(testFp, testNow+testRetention-1, -1))
	d = e.dump()
	assert.Contains(t, d[fk[1]].Fields, "gone")

	require.Nil(t, e.flags(testFp, testNow+testRetention+1, -1))
	d = e.dump()
	assert.NotContains(t, d[fk[1]].Fields, "gone")
	assert.NotContains(t, d[fk[2]].Scores, "gone")
	assert.Contains(t, d[fk[1]].Fields, "live")
	assert.Contains(t, d[fk[2]].Scores, "live")
	assert.NotContains(t, d, fk[3])
}

func TestScriptStaleChangeDoesNotOverwriteNewer(t *testing.T) {
	e := newScriptEnv(t)
	fk := e.keys.FlagKeys()

	require.Nil(t, e.flags(testFp, testNow, 10, ent{"a", 10, false, `{"v":10}`}))
	require.Nil(t, e.flags(testFp, testNow, 10, ent{"a", 5, false, `{"v":5}`}))
	d := e.dump()
	assert.Equal(t, `{"v":10}`, d[fk[1]].Fields["a"])
	assert.Equal(t, float64(10), d[fk[2]].Scores["a"])

	require.Nil(t, e.flags(testFp, testNow, 10, ent{"a", 5, true, `{"v":"removed5"}`}))
	d = e.dump()
	assert.Equal(t, `{"v":10}`, d[fk[1]].Fields["a"])
	assert.NotContains(t, d, fk[3])

	require.Nil(t, e.flags(testFp, testNow, 11, ent{"a", 11, true, `{"v":"removed11"}`}))
	require.Nil(t, e.flags(testFp, testNow, 11, ent{"a", 10, false, `{"v":10}`}))
	d = e.dump()
	assert.Equal(t, `{"v":"removed11"}`, d[fk[1]].Fields["a"])
	assert.Contains(t, d[fk[3]].Scores, "a")
}

func TestScriptSegmentAddRemoveTombstone(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("seg.one")

	require.Nil(t, e.segment(testFp, testNow, "seg.one", 1, []segKey{{"k1", 1}, {"k2", 1}}, nil))
	d := e.dump()
	assert.Equal(t, map[string]float64{"k1": 1, "k2": 1}, d[sk[1]].Scores)
	assert.Equal(t, []string{"seg.one"}, d[sk[4]].Set)
	assert.Equal(t, "1", d[sk[0]].Fields["seg.seg.one.till"])

	require.Nil(t, e.segment(testFp, testNow, "seg.one", 2, nil, []segKey{{"k1", 2}}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k2": 1}, d[sk[1]].Scores)
	assert.Equal(t, map[string]float64{"k1": 2}, d[sk[2]].Scores)
	assert.Equal(t, map[string]float64{"k1": float64(testNow)}, d[sk[3]].Scores)
	assert.Equal(t, "2", d[sk[0]].Fields["seg.seg.one.till"])

	require.Nil(t, e.segment(testFp, testNow, "seg.one", 3, []segKey{{"k1", 3}}, nil))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k1": 3, "k2": 1}, d[sk[1]].Scores)
	assert.NotContains(t, d, sk[2])
	assert.NotContains(t, d, sk[3])

	require.Nil(t, e.segment(testFp, testNow, "seg.one", 5, nil, []segKey{{"k2", 5}}))
	require.Nil(t, e.segment(testFp, testNow, "seg.one", 5, []segKey{{"k2", 4}}, nil))
	d = e.dump()
	assert.NotContains(t, d[sk[1]].Scores, "k2")
	assert.Equal(t, float64(5), d[sk[2]].Scores["k2"])
}

func TestScriptSegmentTombstoneExpires(t *testing.T) {
	e := newScriptEnv(t)
	sk := e.keys.SegmentKeys("s")

	require.Nil(t, e.segment(testFp, testNow, "s", 2, []segKey{{"keep", 1}}, []segKey{{"old", 2}}))
	d := e.dump()
	assert.Contains(t, d[sk[2]].Scores, "old")

	require.Nil(t, e.segment(testFp, testNow+testRetention-1, "s", -1, nil, nil))
	assert.Contains(t, e.dump()[sk[2]].Scores, "old")

	require.Nil(t, e.segment(testFp, testNow+testRetention+1, "s", -1, nil, nil))
	d = e.dump()
	assert.NotContains(t, d, sk[2])
	assert.NotContains(t, d, sk[3])
	assert.Contains(t, d[sk[1]].Scores, "keep")
}

func TestScriptEnvironmentMismatchWritesNothing(t *testing.T) {
	e := newScriptEnv(t)
	require.Nil(t, e.flags(testFp, testNow, 5, ent{"a", 5, false, `{"v":1}`}))
	require.Nil(t, e.segment(testFp, testNow, "s", 1, []segKey{{"k", 1}}, nil))
	before := e.dump()

	err := e.flags("fp-B", testNow, 9, ent{"a", 9, false, `{"v":2}`}, ent{"new", 9, false, `{}`})
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	err = e.rbs("fp-B", testNow, 9, ent{"r", 9, false, `{}`})
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	err = e.segment("fp-B", testNow, "s", 9, []segKey{{"x", 9}}, nil)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, EnvMismatchMarker), err.Error())

	assert.Equal(t, before, e.dump())
}

func TestScriptBadArgsWritesNothing(t *testing.T) {
	e := newScriptEnv(t)

	// Seed valid state so a partial write would be visible as a change
	require.Nil(t, e.flags(testFp, testNow, 1, ent{"seed", 1, false, `{}`}))
	require.Nil(t, e.segment(testFp, testNow, "s", 1, []segKey{{"seedk", 1}}, nil))
	before := e.dump()

	args := entityArgs(testFp, testNow, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	err := e.client.Eval(FlagScript, e.keys.FlagKeys(), args[:len(args)-1]...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	args = entityArgs(testFp, testNow, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[5] = "x"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	// Non-numeric change number and invalid removed flag, both must be refused before any write
	args = entityArgs(testFp, testNow, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[7] = "nope"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	args = entityArgs(testFp, testNow, testRetention, e.keys.FlagCursorField(), 5, ent{"a", 5, false, `{}`})
	args[8] = "2"
	err = e.client.Eval(FlagScript, e.keys.FlagKeys(), args...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	sargs := segmentArgs(testFp, testNow, testRetention, "s", e.keys.SegmentCursorField("s"), 1, []segKey{{"k", 1}}, nil)
	err = e.client.Eval(SegmentScript, e.keys.SegmentKeys("s"), append(sargs, "extra")...)
	require.NotNil(t, err)
	assert.True(t, ErrorHasMarker(err, BadArgsMarker), err.Error())

	sargs = segmentArgs(testFp, testNow, testRetention, "s", e.keys.SegmentCursorField("s"), 1, []segKey{{"k", 1}}, nil)
	sargs[9] = "nope"
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
		require.Nil(t, e.flags(testFp, testNow, cursor))
		require.Nil(t, e.segment(testFp, testNow, "s", cursor, nil, nil))
		d := e.dump()
		assert.Equal(t, step.want, d[fk[0]].Fields["flags.till"], "send %s", step.send)
		assert.Equal(t, step.want, d[sk[0]].Fields["seg.s.till"], "send %s", step.send)
	}
}

func TestScriptCreatesOnlyDocumentedKeys(t *testing.T) {
	e := newScriptEnv(t)

	require.Nil(t, e.flags(testFp, testNow, 3, ent{"f1", 2, false, `{}`}, ent{"f2", 3, true, `{}`}))
	require.Nil(t, e.rbs(testFp, testNow, 3, ent{"r1", 2, false, `{}`}, ent{"r2", 3, true, `{}`}))
	require.Nil(t, e.segment(testFp, testNow, "seg.x", 3, []segKey{{"k1", 1}, {"k2", 1}}, []segKey{{"k2", 3}}))

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

	require.Nil(t, e.segment(testFp, testNow, "s", 2, nil, []segKey{{"k", 2}}))
	d := e.dump()
	assert.Equal(t, map[string]float64{"k": float64(testNow)}, d[sk[3]].Scores, fmt.Sprint(d[sk[3]]))

	require.Nil(t, e.segment(testFp, testNow+500, "s", 2, nil, []segKey{{"k", 2}}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"k": float64(testNow)}, d[sk[3]].Scores, "resend must not refresh the segment tombstone clock")

	require.Nil(t, e.flags(testFp, testNow, 2, ent{"f", 2, true, `{}`}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"f": float64(testNow)}, d[fk[3]].Scores)

	require.Nil(t, e.flags(testFp, testNow+500, 2, ent{"f", 2, true, `{}`}))
	d = e.dump()
	assert.Equal(t, map[string]float64{"f": float64(testNow)}, d[fk[3]].Scores, "resend must not refresh the flag tombstone clock")
}

func TestResetOnlyDeletesOwnNamespace(t *testing.T) {
	a := newScriptEnv(t)
	b := a.with("other-env")

	require.Nil(t, a.flags(testFp, testNow, 1, ent{"f", 1, false, `{}`}))
	require.Nil(t, a.segment(testFp, testNow, "s", 1, []segKey{{"k", 1}}, nil))
	require.Nil(t, b.flags(testFp, testNow, 1, ent{"f", 1, false, `{}`}))
	require.Nil(t, a.client.Set("SPLITIO.split.x", "value", 0))

	require.Nil(t, a.client.Eval(ResetScript, nil, a.keys.Pattern()))

	for _, name := range a.names() {
		assert.NotContains(t, name, "{SPLITIO_PROXY."+a.keys.Env()+"}", name)
	}
	assert.Contains(t, a.names(), b.keys.FlagKeys()[1])
	assert.Contains(t, a.names(), a.prefix+".SPLITIO.split.x")
	assert.Len(t, a.names(), 4)
}
