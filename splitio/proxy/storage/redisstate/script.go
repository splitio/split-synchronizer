package redisstate

import "strings"

const (
	// EnvMismatchMarker is the error text a script returns when the stored fingerprint differs from the writer's
	EnvMismatchMarker = "PROXY_ENV_MISMATCH"
	// BadArgsMarker is the error text a script returns when its argument count is inconsistent
	BadArgsMarker = "PROXY_BAD_ARGS"
)

// ErrorHasMarker reports whether err carries the given script refusal marker
func ErrorHasMarker(err error, marker string) bool {
	return err != nil && strings.Contains(err.Error(), marker)
}

// FlagScript atomically upserts feature flags (or any entity set) and their cursor.
// KEYS come from Keys.FlagKeys. It validates arguments and the fingerprint before writing anything.
const FlagScript = `-- KEYS: 1 meta, 2 data hash, 3 index zset, 4 removed zset
-- ARGV: 1 fingerprint, 2 nowMs, 3 retentionMs, 4 cursorField, 5 cursor, 6 n, then per entity 4 values: name, cn, removed(0|1), json
local n = tonumber(ARGV[6])
if n == nil or #ARGV ~= 6 + n * 4 then
  return redis.error_reply('PROXY_BAD_ARGS')
end
if tonumber(ARGV[2]) == nil or tonumber(ARGV[3]) == nil or tonumber(ARGV[5]) == nil then
  return redis.error_reply('PROXY_BAD_ARGS')
end
for i = 0, n - 1 do
  local b = 7 + i * 4
  if tonumber(ARGV[b + 1]) == nil or (ARGV[b + 2] ~= '0' and ARGV[b + 2] ~= '1') then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
local fp = redis.call('HGET', KEYS[1], 'fingerprint')
if fp and fp ~= ARGV[1] then
  return redis.error_reply('PROXY_ENV_MISMATCH')
end
if not fp then
  redis.call('HSET', KEYS[1], 'fingerprint', ARGV[1])
end
for i = 0, n - 1 do
  local b = 7 + i * 4
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
      redis.call('ZADD', KEYS[4], tonumber(ARGV[2]), name)
    else
      redis.call('ZREM', KEYS[4], name)
    end
  end
end
local till = tonumber(redis.call('HGET', KEYS[1], ARGV[4]) or '-1')
if tonumber(ARGV[5]) > till then
  redis.call('HSET', KEYS[1], ARGV[4], ARGV[5])
end
local cutoff = tonumber(ARGV[2]) - tonumber(ARGV[3])
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
const SegmentScript = `-- KEYS: 1 meta, 2 active zset, 3 removed zset, 4 segat zset (removal ms), 5 segments set
-- ARGV: 1 fingerprint, 2 nowMs, 3 retentionMs, 4 name, 5 cursorField, 6 cursor, 7 nAdd, 8 nRemove, then nAdd pairs (key, cn), then nRemove pairs (key, cn)
local nAdd = tonumber(ARGV[7])
local nRem = tonumber(ARGV[8])
if nAdd == nil or nRem == nil or #ARGV ~= 8 + 2 * (nAdd + nRem) then
  return redis.error_reply('PROXY_BAD_ARGS')
end
local base = 8 + 2 * nAdd
if tonumber(ARGV[2]) == nil or tonumber(ARGV[3]) == nil or tonumber(ARGV[6]) == nil then
  return redis.error_reply('PROXY_BAD_ARGS')
end
for i = 0, nAdd - 1 do
  if tonumber(ARGV[10 + 2 * i]) == nil then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
for i = 0, nRem - 1 do
  if tonumber(ARGV[base + 2 + 2 * i]) == nil then
    return redis.error_reply('PROXY_BAD_ARGS')
  end
end
local fp = redis.call('HGET', KEYS[1], 'fingerprint')
if fp and fp ~= ARGV[1] then
  return redis.error_reply('PROXY_ENV_MISMATCH')
end
if not fp then
  redis.call('HSET', KEYS[1], 'fingerprint', ARGV[1])
end
local now = tonumber(ARGV[2])
redis.call('SADD', KEYS[5], ARGV[4])
for i = 0, nAdd - 1 do
  local key = ARGV[9 + 2 * i]
  local cn = tonumber(ARGV[10 + 2 * i])
  local cur = math.max(tonumber(redis.call('ZSCORE', KEYS[2], key) or '-1'), tonumber(redis.call('ZSCORE', KEYS[3], key) or '-1'))
  if cn >= cur then
    redis.call('ZADD', KEYS[2], cn, key)
    redis.call('ZREM', KEYS[3], key)
    redis.call('ZREM', KEYS[4], key)
  end
end
for i = 0, nRem - 1 do
  local key = ARGV[base + 1 + 2 * i]
  local cn = tonumber(ARGV[base + 2 + 2 * i])
  local cur = math.max(tonumber(redis.call('ZSCORE', KEYS[2], key) or '-1'), tonumber(redis.call('ZSCORE', KEYS[3], key) or '-1'))
  local apply = cn > cur
  if (not apply) and cn == cur then
    apply = (redis.call('ZSCORE', KEYS[3], key) == false)
  end
  if apply then
    redis.call('ZADD', KEYS[3], cn, key)
    redis.call('ZADD', KEYS[4], now, key)
    redis.call('ZREM', KEYS[2], key)
  end
end
local till = tonumber(redis.call('HGET', KEYS[1], ARGV[5]) or '-1')
if tonumber(ARGV[6]) > till then
  redis.call('HSET', KEYS[1], ARGV[5], ARGV[6])
end
local cutoff = now - tonumber(ARGV[3])
local expired = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', cutoff, 'LIMIT', 0, 1000)
for _, key in ipairs(expired) do
  redis.call('ZREM', KEYS[3], key)
  redis.call('ZREM', KEYS[4], key)
end
return 1
`

// ResetScript deletes every key matching ARGV[1] (use Keys.Pattern) and returns the number of deleted keys
const ResetScript = `local keys = redis.call('KEYS', ARGV[1]) for _, k in ipairs(keys) do redis.call('DEL', k) end return #keys`
