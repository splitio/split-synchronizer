package redisstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	config "github.com/splitio/go-split-commons/v10/conf"
)

// LeaseCallBound is the deadline of every lease call (acquire, renew, release). It is named and fixed, not configurable:
// the minimum lease TTL is three times this bound so one renew always fits in its interval.
const LeaseCallBound = 2 * time.Second

var (
	// ErrLeaseHeld is returned by Acquire when another owner holds the lease key. The caller stays a non-writer.
	ErrLeaseHeld = errors.New("redis state writer lease is held by another proxy")
	// ErrLeaseLost is returned by Renew when the lease key no longer belongs to the caller (expired, released or taken
	// over). The caller must stop writing at once.
	ErrLeaseLost = errors.New("redis state writer lease is not held by this proxy")
)

// AcquireScript takes the writer lease and returns {token, 1} when acquired or {0, 0} when another owner holds it.
// KEYS: 1 lease, 2 meta. ARGV: 1 owner, 2 fingerprint, 3 ttlMs, 4 newTerm (0|1), 5 forceFresh (0|1).
// Order: checkMeta, foreign fingerprint refusal (unless forceFresh), lease check (held returns {0, 0} and writes
// nothing), then write an absent fingerprint, fence bump or same-term reuse, SET lease PX. A refused call writes nothing.
const AcquireScript = luaPrelude + `if ARGV[1] == nil or ARGV[1] == '' or ARGV[2] == nil or ARGV[2] == '' or not isint(ARGV[3]) or tonumber(ARGV[3]) <= 0 or (ARGV[4] ~= '0' and ARGV[4] ~= '1') or (ARGV[5] ~= '0' and ARGV[5] ~= '1') then
  error('PROXY_BAD_ARGS')
end
checkMeta(KEYS[2])
local stored = redis.call('HGET', KEYS[2], 'fingerprint')
if stored ~= false and stored ~= ARGV[2] and ARGV[5] ~= '1' then
  error('PROXY_ENV_MISMATCH')
end
local cur = redis.call('GET', KEYS[1])
if cur ~= false and cur ~= ARGV[1] then
  return {0, 0}
end
if stored == false then
  redis.call('HSET', KEYS[2], 'fingerprint', ARGV[2])
end
local token = false
if cur ~= false and ARGV[4] == '0' then
  token = redis.call('HGET', KEYS[2], 'fence')
end
if not token then
  token = redis.call('HINCRBY', KEYS[2], 'fence', 1)
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
return {tonumber(token), 1}
`

// RenewScript extends the lease when the caller still owns it and returns the current fence.
// KEYS: 1 lease, 2 meta. ARGV: 1 owner, 2 ttlMs. A foreign or missing lease raises PROXY_FENCED.
const RenewScript = luaPrelude + `if ARGV[1] == nil or ARGV[1] == '' or not isint(ARGV[2]) or tonumber(ARGV[2]) <= 0 then
  error('PROXY_BAD_ARGS')
end
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  error('PROXY_FENCED')
end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return tonumber(redis.call('HGET', KEYS[2], 'fence'))
`

// ReleaseScript deletes the lease only when it belongs to the owner AND the stored fence equals the token, so a late
// release from an older term never deletes a newer term's lease. Returns 1 when deleted, 0 otherwise.
// KEYS: 1 lease, 2 meta. ARGV: 1 owner, 2 token.
const ReleaseScript = luaPrelude + `if ARGV[1] == nil or ARGV[1] == '' or not isint(ARGV[2]) then
  error('PROXY_BAD_ARGS')
end
if redis.call('GET', KEYS[1]) == ARGV[1] and tonumber(redis.call('HGET', KEYS[2], 'fence')) == tonumber(ARGV[2]) then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// LeaseClient runs the writer lease scripts on a dedicated go-redis client. It is separate from the writer and reader
// clients so its 2s deadlines and zero retries do not inherit the long connection timeouts.
type LeaseClient struct {
	client goredis.UniversalClient
}

// NewLeaseClient builds the lease client from the Redis config. Cluster mode is rejected, as for the reader.
func NewLeaseClient(cfg *config.RedisConfig) (*LeaseClient, error) {
	opts, err := leaseOptions(cfg)
	if err != nil {
		return nil, err
	}
	return &LeaseClient{client: goredis.NewUniversalClient(opts)}, nil
}

// leaseOptions maps the config like universalOptions, then overrides the timeouts and retries for lease calls.
func leaseOptions(cfg *config.RedisConfig) (*goredis.UniversalOptions, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: nil config", ErrInvalidReadConfig)
	}
	opts, _, err := universalOptions(cfg)
	if err != nil {
		return nil, err
	}
	opts.DialTimeout = LeaseCallBound
	opts.ReadTimeout = LeaseCallBound
	opts.WriteTimeout = LeaseCallBound
	// go-redis treats MaxRetries 0 as the default of 3; -1 disables retries.
	opts.MaxRetries = -1
	opts.ContextTimeoutEnabled = true
	return opts, nil
}

// withBound returns a context that ends after LeaseCallBound.
func withBound(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, LeaseCallBound)
}

// Acquire takes the lease for owner and returns the fencing token of this term. ErrLeaseHeld means another owner holds
// the lease; ErrEnvironmentMismatch means a foreign fingerprint was refused (forceFresh false). newTerm bumps the fence
// even when the lease key still exists for owner; forceFresh lets a foreign fingerprint through without rewriting it.
func (c *LeaseClient) Acquire(ctx context.Context, keys Keys, owner, fingerprint string, ttl time.Duration, newTerm, forceFresh bool) (int64, error) {
	ctx, cancel := withBound(ctx)
	defer cancel()
	res, err := c.client.Eval(ctx, AcquireScript, []string{keys.Lease(), keys.Meta()},
		owner, fingerprint, ttl.Milliseconds(), boolArg(newTerm), boolArg(forceFresh)).Slice()
	if err != nil {
		if ErrorHasMarker(err, EnvMismatchMarker) {
			return 0, ErrEnvironmentMismatch
		}
		return 0, fmt.Errorf("lease acquire: %w", err)
	}
	if len(res) != 2 {
		return 0, fmt.Errorf("lease acquire: unexpected reply of %d values", len(res))
	}
	token, _ := res[0].(int64)
	acquired, _ := res[1].(int64)
	if acquired == 0 {
		return 0, ErrLeaseHeld
	}
	return token, nil
}

// Renew extends the lease for owner and returns the current fence. ErrLeaseLost means the lease is no longer owned by
// owner, and the caller must step down.
func (c *LeaseClient) Renew(ctx context.Context, keys Keys, owner string, ttl time.Duration) (int64, error) {
	ctx, cancel := withBound(ctx)
	defer cancel()
	fence, err := c.client.Eval(ctx, RenewScript, []string{keys.Lease(), keys.Meta()}, owner, ttl.Milliseconds()).Int64()
	if err != nil {
		if ErrorHasMarker(err, FencedMarker) {
			return 0, ErrLeaseLost
		}
		return 0, fmt.Errorf("lease renew: %w", err)
	}
	return fence, nil
}

// Release deletes the lease when it still belongs to owner at this token. A no-op is not an error. The caller logs and
// ignores a returned error; the fenced scripts still reject stale writers.
func (c *LeaseClient) Release(ctx context.Context, keys Keys, owner string, token int64) error {
	ctx, cancel := withBound(ctx)
	defer cancel()
	if err := c.client.Eval(ctx, ReleaseScript, []string{keys.Lease(), keys.Meta()}, owner, token).Err(); err != nil {
		return fmt.Errorf("lease release: %w", err)
	}
	return nil
}

// Close releases the connection pool. The caller owns the client.
func (c *LeaseClient) Close() error {
	return c.client.Close()
}

func boolArg(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
