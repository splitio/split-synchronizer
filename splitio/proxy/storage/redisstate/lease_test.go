package redisstate

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/splitio/go-split-commons/v10/conf"
)

// leaseTestTTL is short so the expiry tests stay fast; only the tests that need expiry sleep.
const leaseTestTTL = 300 * time.Millisecond

// newLeaseTestEnv returns a lease client and the keys of a private prefix on the Redis at localhost:6379. Keys of the
// prefix are wiped before the test and at cleanup. Skips (or fails in CI) when Redis is not reachable.
func newLeaseTestEnv(t *testing.T) (*LeaseClient, Keys) {
	t.Helper()
	requireRedis(t)
	prefix := "lease-" + t.Name()
	cfg := &config.RedisConfig{Host: "localhost", Port: 6379, Database: 0, Network: "tcp", Prefix: prefix}
	lc, err := NewLeaseClient(cfg)
	require.NoError(t, err)
	wipe := func() { _ = lc.client.Eval(context.Background(), realRedisWipeScript, nil, prefix+".*").Err() }
	wipe()
	t.Cleanup(func() {
		wipe()
		_ = lc.Close()
	})
	keys, err := NewKeys(prefix, "lease-key", "")
	require.NoError(t, err)
	return lc, keys
}

// TestLeaseAcquireOnlyOneWins covers T1: one owner holds the lease, a second owner is refused, renew and release work for
// the holder, and the lease is free again after release.
func TestLeaseAcquireOnlyOneWins(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	tokenA, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), tokenA)

	_, err = lc.Acquire(ctx, keys, "owner-b", "fp", time.Minute, false, false)
	assert.ErrorIs(t, err, ErrLeaseHeld)

	fence, err := lc.Renew(ctx, keys, "owner-a", time.Minute)
	require.NoError(t, err)
	assert.Equal(t, tokenA, fence)

	require.NoError(t, lc.Release(ctx, keys, "owner-a", tokenA))
	assert.Equal(t, int64(0), lc.client.Exists(ctx, keys.Lease()).Val())

	tokenB, err := lc.Acquire(ctx, keys, "owner-b", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, tokenA+1, tokenB)
}

// TestLeaseCallBoundHungCall covers T3: a lease call against a server that accepts and never replies returns an error
// within about LeaseCallBound, not within the connection timeouts. No Redis is needed.
func TestLeaseCallBoundHungCall(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	cfg := &config.RedisConfig{Host: "127.0.0.1", Port: port, Database: 0, Network: "tcp", Prefix: "lease-hung"}
	lc, err := NewLeaseClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lc.Close() })
	keys, err := NewKeys("lease-hung", "lease-key", "")
	require.NoError(t, err)

	start := time.Now()
	_, err = lc.Acquire(context.Background(), keys, "owner-a", "fp", time.Minute, false, false)
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Less(t, elapsed, LeaseCallBound+time.Second)
}

// TestLeaseExpiryAllowsTakeover covers T3: after the lease expires another owner takes it, and the expired holder gets a
// higher token when it acquires again.
func TestLeaseExpiryAllowsTakeover(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	tokenA, err := lc.Acquire(ctx, keys, "owner-a", "fp", leaseTestTTL, false, false)
	require.NoError(t, err)
	time.Sleep(leaseTestTTL + 100*time.Millisecond)

	tokenB, err := lc.Acquire(ctx, keys, "owner-b", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Greater(t, tokenB, tokenA)

	_, err = lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	assert.ErrorIs(t, err, ErrLeaseHeld)
}

// TestLeaseRenewOwnerMismatch covers T4: renew by a non-owner returns ErrLeaseLost and does not extend the lease.
func TestLeaseRenewOwnerMismatch(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	_, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)

	_, err = lc.Renew(ctx, keys, "owner-b", time.Minute)
	assert.ErrorIs(t, err, ErrLeaseLost)
	_, err = lc.Renew(ctx, keys, "owner-a", time.Minute)
	assert.NoError(t, err)
}

// TestFailoverLostLeaseAndFenceStaleReleaseIsNoop covers T8 (lease side): after the lease and the fence are lost, a new
// acquire gets a token of at least 1, and the stale owner's release does not delete the new owner's lease.
func TestFailoverLostLeaseAndFenceStaleReleaseIsNoop(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	staleToken, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)
	require.NoError(t, lc.client.Del(ctx, keys.Lease()).Err())
	require.NoError(t, lc.client.HDel(ctx, keys.Meta(), FenceField).Err())

	tokenB, err := lc.Acquire(ctx, keys, "owner-b", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tokenB, int64(1))

	require.NoError(t, lc.Release(ctx, keys, "owner-a", staleToken))
	assert.Equal(t, "owner-b", lc.client.Get(ctx, keys.Lease()).Val())
}

// TestLeaseAcquireWritesFingerprint covers T15 (lease side): an acquire on an empty namespace writes the fingerprint, so
// readers never see a meta hash without it.
func TestLeaseAcquireWritesFingerprint(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	_, err := lc.Acquire(ctx, keys, "owner-a", "fp-mine", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, "fp-mine", lc.client.HGet(ctx, keys.Meta(), FingerprintField).Val())
}

// TestLeaseAcquireRefusesForeignFingerprint covers T16: a foreign fingerprint is refused with ErrEnvironmentMismatch when
// forceFresh is 0, and the lease key is not created.
func TestLeaseAcquireRefusesForeignFingerprint(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()
	require.NoError(t, lc.client.HSet(ctx, keys.Meta(), FingerprintField, "fp-other", FenceField, 5).Err())

	_, err := lc.Acquire(ctx, keys, "owner-a", "fp-mine", time.Minute, false, false)
	assert.ErrorIs(t, err, ErrEnvironmentMismatch)
	assert.Equal(t, int64(0), lc.client.Exists(ctx, keys.Lease()).Val())
	assert.Equal(t, "fp-other", lc.client.HGet(ctx, keys.Meta(), FingerprintField).Val())
}

// TestLeaseAcquireForceFreshTakesForeignFingerprint covers the forceFresh exception of T14 and D2: with forceFresh 1 a
// foreign fingerprint is taken over, the fence is bumped, and the fingerprint is NOT written by the acquire.
func TestLeaseAcquireForceFreshTakesForeignFingerprint(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()
	require.NoError(t, lc.client.HSet(ctx, keys.Meta(), FingerprintField, "fp-other", FenceField, 5).Err())

	token, err := lc.Acquire(ctx, keys, "owner-a", "fp-mine", time.Minute, false, true)
	require.NoError(t, err)
	assert.Equal(t, int64(6), token)
	assert.Equal(t, "fp-other", lc.client.HGet(ctx, keys.Meta(), FingerprintField).Val())
}

// TestAcquireRetryAfterLostReplySameToken covers T18 and T25: a same-term retry returns the same token while the lease
// key exists for the owner, newTerm bumps it, an expired lease bumps it, and a wrong-type meta key takes no lease.
func TestAcquireRetryAfterLostReplySameToken(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	token, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), token)

	retry, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, token, retry)

	newTerm, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, true, false)
	require.NoError(t, err)
	assert.Equal(t, token+1, newTerm)

	require.NoError(t, lc.client.Del(ctx, keys.Lease()).Err())
	afterRelease, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)
	assert.Equal(t, newTerm+1, afterRelease)

	// A wrong-type meta key is refused before any lease is written.
	require.NoError(t, lc.client.Del(ctx, keys.Lease(), keys.Meta()).Err())
	require.NoError(t, lc.client.Set(ctx, keys.Meta(), "not-a-hash", 0).Err())
	_, err = lc.Acquire(ctx, keys, "owner-c", "fp", time.Minute, false, false)
	assert.ErrorContains(t, err, MetaWrongTypeMarker)
	assert.Equal(t, int64(0), lc.client.Exists(ctx, keys.Lease()).Val())
}

// TestLeaseAcquireBadArgs covers T19 and FL-16: a non-integer ttl is refused with PROXY_BAD_ARGS. The Go API takes a
// Duration, so the script is called directly with the raw argument.
func TestLeaseAcquireBadArgs(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	err := lc.client.Eval(ctx, AcquireScript, []string{keys.Lease(), keys.Meta()}, "owner-a", "fp", "1.5", "0", "0").Err()
	assert.ErrorContains(t, err, BadArgsMarker)
	assert.Equal(t, int64(0), lc.client.Exists(ctx, keys.Lease()).Val())
}

// TestLeaseReleaseOnlyOwnerAndToken covers T20 and FL-22: release deletes the lease only for the owner at the stored
// token. A wrong token or another owner leaves the lease in place.
func TestLeaseReleaseOnlyOwnerAndToken(t *testing.T) {
	lc, keys := newLeaseTestEnv(t)
	ctx := context.Background()

	token, err := lc.Acquire(ctx, keys, "owner-a", "fp", time.Minute, false, false)
	require.NoError(t, err)

	require.NoError(t, lc.Release(ctx, keys, "owner-a", token+1))
	require.NoError(t, lc.Release(ctx, keys, "owner-b", token))
	assert.Equal(t, int64(1), lc.client.Exists(ctx, keys.Lease()).Val())

	require.NoError(t, lc.Release(ctx, keys, "owner-a", token))
	assert.Equal(t, int64(0), lc.client.Exists(ctx, keys.Lease()).Val())
}

// TestNewLeaseClientOptions covers T28 and T30: the lease client overrides the timeouts to LeaseCallBound and disables
// retries whatever the connection config says, keeps the same addresses as the reader, and rejects cluster mode.
func TestNewLeaseClientOptions(t *testing.T) {
	cfg := &config.RedisConfig{Host: "redis.local", Port: 6380, Database: 0, Network: "tcp", Prefix: "p",
		DialTimeout: 30, ReadTimeout: 30, WriteTimeout: 30, MaxRetries: 5}

	opts, err := leaseOptions(cfg)
	require.NoError(t, err)
	assert.Equal(t, LeaseCallBound, opts.DialTimeout)
	assert.Equal(t, LeaseCallBound, opts.ReadTimeout)
	assert.Equal(t, LeaseCallBound, opts.WriteTimeout)
	assert.Equal(t, -1, opts.MaxRetries)
	assert.True(t, opts.ContextTimeoutEnabled)

	readOpts, _, err := universalOptions(cfg)
	require.NoError(t, err)
	assert.Equal(t, readOpts.Addrs, opts.Addrs)

	_, err = leaseOptions(&config.RedisConfig{ClusterNodes: []string{"a:1"}})
	assert.ErrorIs(t, err, ErrInvalidReadConfig)
	_, err = leaseOptions(nil)
	assert.ErrorIs(t, err, ErrInvalidReadConfig)
}
