package redisstate

import "strings"

const (
	// EnvMismatchMarker is the error text a script returns when the stored fingerprint differs from the writer's
	EnvMismatchMarker = "PROXY_ENV_MISMATCH"
	// BadArgsMarker is the error text a script returns when its argument count is inconsistent
	BadArgsMarker = "PROXY_BAD_ARGS"
	// MetaWrongTypeMarker is the error text a script returns when the meta hash key holds another type. Unlike a plain
	// WRONGTYPE (an entity key), it is a round failure: the writer retries it and skips nothing.
	MetaWrongTypeMarker = "PROXY_META_WRONGTYPE"
)

// wrongTypeMarker is the Redis error for a key that holds another type. Retrying can not fix it, so the writer treats
// it like a refused chunk: the entity is isolated, skipped instead of stalling the queue.
const wrongTypeMarker = "WRONGTYPE"

// ErrorHasMarker reports whether err carries the given script refusal marker
func ErrorHasMarker(err error, marker string) bool {
	return err != nil && strings.Contains(err.Error(), marker)
}

// luaPrelude defines the helpers every script shares: isint accepts only optionally signed decimal integers (so
// "1.5", "1e3" and "" are refused) and nowMs reads the Redis clock (D23), so the removal clock is the one Redis clock.
const luaPrelude = `local function isint(s)
  return s ~= false and s ~= nil and string.match(s, '^%-?%d+$') ~= nil
end
local function nowMs()
  local t = redis.call('TIME')
  return tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
end
local function checkMeta(key)
  local ty = redis.call('TYPE', key)['ok']
  if ty ~= 'hash' and ty ~= 'none' then
    error('PROXY_META_WRONGTYPE')
  end
end
`

// FlagScript atomically upserts feature flags (or any entity set) and their cursor.
// KEYS come from Keys.FlagKeys. It validates arguments and the fingerprint before writing anything.
const FlagScript = luaPrelude + `-- KEYS: 1 meta, 2 data hash, 3 index zset, 4 removed zset
-- ARGV: 1 fingerprint, 2 retentionMs, 3 cursorField, 4 cursor, 5 n, then per entity 4 values: name, cn, removed(0|1), json
if not isint(ARGV[2]) or tonumber(ARGV[2]) < 0 or not isint(ARGV[4]) or not isint(ARGV[5]) then
  return redis.error_reply('PROXY_BAD_ARGS')
end
local n = tonumber(ARGV[5])
if n < 0 or #ARGV ~= 5 + n * 4 then
  return redis.error_reply('PROXY_BAD_ARGS')
end
for i = 0, n - 1 do
  local b = 6 + i * 4
  if not isint(ARGV[b + 1]) or (ARGV[b + 2] ~= '0' and ARGV[b + 2] ~= '1') then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
local now = nowMs()
checkMeta(KEYS[1])
local fp = redis.call('HGET', KEYS[1], 'fingerprint')
if fp and fp ~= ARGV[1] then
  return redis.error_reply('PROXY_ENV_MISMATCH')
end
if not fp then
  redis.call('HSET', KEYS[1], 'fingerprint', ARGV[1])
end
redis.call('HSET', KEYS[1], 'updatedAtMs', now)
for i = 0, n - 1 do
  local b = 6 + i * 4
  local name = ARGV[b]
  local cn = tonumber(ARGV[b + 1])
  local cur = tonumber(redis.call('ZSCORE', KEYS[3], name) or '-1')
  local removed = ARGV[b + 2] == '1'
  local apply = cn > cur
  if (not apply) and cn == cur then
    if removed then
      apply = (redis.call('ZSCORE', KEYS[4], name) == false)
    else
      apply = true
    end
  end
  if apply then
    redis.call('HSET', KEYS[2], name, ARGV[b + 3])
    redis.call('ZADD', KEYS[3], cn, name)
    if removed then
      redis.call('ZADD', KEYS[4], now, name)
    else
      redis.call('ZREM', KEYS[4], name)
    end
  end
end
if redis.call('HEXISTS', KEYS[1], ARGV[3]) == 0 then
  redis.call('HSET', KEYS[1], ARGV[3], '-1')
end
local till = tonumber(redis.call('HGET', KEYS[1], ARGV[3]))
if tonumber(ARGV[4]) > till then
  redis.call('HSET', KEYS[1], ARGV[3], ARGV[4])
end
local cutoff = now - tonumber(ARGV[2])
local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', cutoff, 'LIMIT', 0, 100)
for _, name in ipairs(expired) do
  redis.call('HDEL', KEYS[2], name)
  redis.call('ZREM', KEYS[3], name)
  redis.call('ZREM', KEYS[4], name)
end
return 1
`

// RuleBasedSegmentScript is the entity script applied to rule-based segments (KEYS from Keys.RuleBasedSegmentKeys)
const RuleBasedSegmentScript = FlagScript

// SegmentScript atomically applies key additions, removals (tombstones) and the cursor of one segment.
// KEYS come from Keys.SegmentKeys.
const SegmentScript = luaPrelude + `-- KEYS: 1 meta, 2 active zset, 3 removed zset, 4 segat zset (removal ms)
-- ARGV: 1 fingerprint, 2 retentionMs, 3 name, 4 cursorField, 5 cursor, 6 nAdd, 7 nRemove, then nAdd pairs (key, cn), then nRemove pairs (key, cn)
local nAdd = tonumber(ARGV[6])
local nRem = tonumber(ARGV[7])
if not isint(ARGV[2]) or tonumber(ARGV[2]) < 0 or not isint(ARGV[5]) or not isint(ARGV[6]) or not isint(ARGV[7]) then
  return redis.error_reply('PROXY_BAD_ARGS')
end
if nAdd < 0 or nRem < 0 or #ARGV ~= 7 + 2 * (nAdd + nRem) then
  return redis.error_reply('PROXY_BAD_ARGS')
end
local base = 7 + 2 * nAdd
for i = 0, nAdd - 1 do
  if not isint(ARGV[9 + 2 * i]) then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
for i = 0, nRem - 1 do
  if not isint(ARGV[base + 2 + 2 * i]) then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
local now = nowMs()
checkMeta(KEYS[1])
local fp = redis.call('HGET', KEYS[1], 'fingerprint')
if fp and fp ~= ARGV[1] then
  return redis.error_reply('PROXY_ENV_MISMATCH')
end
if not fp then
  redis.call('HSET', KEYS[1], 'fingerprint', ARGV[1])
end
redis.call('HSET', KEYS[1], 'updatedAtMs', now)
if redis.call('HEXISTS', KEYS[1], ARGV[4]) == 0 then
  redis.call('HSET', KEYS[1], ARGV[4], '-1')
end
for i = 0, nAdd - 1 do
  local key = ARGV[8 + 2 * i]
  local cn = tonumber(ARGV[9 + 2 * i])
  local cur = math.max(tonumber(redis.call('ZSCORE', KEYS[2], key) or '-1'), tonumber(redis.call('ZSCORE', KEYS[3], key) or '-1'))
  if cn > cur then
    redis.call('ZADD', KEYS[2], cn, key)
    redis.call('ZREM', KEYS[3], key)
    redis.call('ZREM', KEYS[4], key)
  end
end
for i = 0, nRem - 1 do
  local key = ARGV[base + 1 + 2 * i]
  local cn = tonumber(ARGV[base + 2 + 2 * i])
  local cur = math.max(tonumber(redis.call('ZSCORE', KEYS[2], key) or '-1'), tonumber(redis.call('ZSCORE', KEYS[3], key) or '-1'))
  -- D37: on a change number tie the removal wins; a tombstone already at cn is not rewritten
  if cn > cur or (cn == cur and redis.call('ZSCORE', KEYS[3], key) == false) then
    redis.call('ZADD', KEYS[3], cn, key)
    redis.call('ZADD', KEYS[4], now, key)
    redis.call('ZREM', KEYS[2], key)
  end
end
local till = tonumber(redis.call('HGET', KEYS[1], ARGV[4]))
if tonumber(ARGV[5]) > till then
  redis.call('HSET', KEYS[1], ARGV[4], ARGV[5])
end
local cutoff = now - tonumber(ARGV[2])
local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', cutoff, 'LIMIT', 0, 1000)
for _, key in ipairs(expired) do
  redis.call('ZREM', KEYS[3], key)
  redis.call('ZREM', KEYS[4], key)
end
return 1
`

// countMismatchMarker is the error marker SegmentCheckScript returns when Redis does not hold the expected segment
const countMismatchMarker = "PROXY_COUNT_MISMATCH"

// SegmentCheckScript fails with PROXY_COUNT_MISMATCH unless the cursor field of the segment (D22) is present and not
// -1, and its active key count is ARGV[1]. KEYS come from Keys.SegmentKeys, ARGV[2] is the segment name.
const SegmentCheckScript = `-- KEYS: 1 meta, 2 active zset, 3 removed zset, 4 segat zset
local till = redis.call('HGET', KEYS[1], 'seg.' .. ARGV[2] .. '.till')
if not till or tonumber(till) < 0 or redis.call('ZCARD', KEYS[2]) ~= tonumber(ARGV[1]) then
  return redis.error_reply('PROXY_COUNT_MISMATCH')
end
return 1
`

// SegmentDropScript deletes a segment Redis holds but the local state no longer lists: its three keys and its cursor
// field (D40, T163). KEYS come from Keys.SegmentKeys, ARGV[1] is the segment name.
const SegmentDropScript = luaPrelude + `-- KEYS: 1 meta, 2 active zset, 3 removed zset, 4 segat zset
checkMeta(KEYS[1])
redis.call('DEL', KEYS[2], KEYS[3], KEYS[4])
redis.call('HDEL', KEYS[1], 'seg.' .. ARGV[1] .. '.till')
return 1
`

// PurgeScript removes up to ARGV[3] expired removal markers from one index: the entries of the removal clock zset
// (KEYS[4]) older than the retention (ARGV[2]) leave the index (KEYS[3]) and the clock, and for flags and rule-based
// segments (ARGV[4] = 1) their data (KEYS[2]). KEYS come from Keys.FlagKeys, Keys.RuleBasedSegmentKeys or
// Keys.SegmentKeys (segments: KEYS[3] tombstones, KEYS[4] removal clock; KEYS[2] is not touched).
// It checks the fingerprint like the write scripts (an absent fingerprint is set, a foreign one is refused) and
// writes the number of entries it removed to the string key KEYS[5] (Keys.PurgeLast, optional), so the Writer can repeat the call
// while the count equals the batch. The Client Eval returns no result (D27).
const PurgeScript = luaPrelude + `-- KEYS: 1 meta, 2 data hash, 3 index zset, 4 removal clock zset, 5 purge count key (optional)
-- ARGV: 1 fingerprint, 2 retentionMs, 3 batch, 4 hasData (0|1)
if #ARGV ~= 4 or not isint(ARGV[2]) or tonumber(ARGV[2]) < 0 or not isint(ARGV[3]) or tonumber(ARGV[3]) <= 0 then
  return redis.error_reply('PROXY_BAD_ARGS')
end
if ARGV[4] ~= '0' and ARGV[4] ~= '1' then
  return redis.error_reply('PROXY_BAD_ARGS')
end
local now = nowMs()
checkMeta(KEYS[1])
local fp = redis.call('HGET', KEYS[1], 'fingerprint')
if fp and fp ~= ARGV[1] then
  return redis.error_reply('PROXY_ENV_MISMATCH')
end
if not fp then
  redis.call('HSET', KEYS[1], 'fingerprint', ARGV[1])
end
local cutoff = now - tonumber(ARGV[2])
local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', cutoff, 'LIMIT', 0, tonumber(ARGV[3]))
for _, name in ipairs(expired) do
  if ARGV[4] == '1' then
    redis.call('HDEL', KEYS[2], name)
  end
  redis.call('ZREM', KEYS[3], name)
  redis.call('ZREM', KEYS[4], name)
end
if KEYS[5] then redis.call('SET', KEYS[5], #expired) end
return 1
`

// startProbeScript is the Start probe (D30). It reads TIME and then writes, the path that needs Redis 5.0 effects
// replication; on an older server it fails with a script error, which Start reports as a minimum-version InitError.
// KEYS: 1 probe key (the writer deletes it again in the same script).
const startProbeScript = `redis.call('TIME')
redis.call('SET', KEYS[1], '1')
redis.call('DEL', KEYS[1])
return 1
`
