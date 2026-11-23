package redisstate

import (
	"context"
	"testing"
	"time"

	commonsredis "github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fencedWriterSetup starts a real Redis, seeds the lease for testOwner at testToken and returns a started-or-not Writer
// on an eval counter. The lease is moved to another owner by fencedWriterMoveLease, so the next script call is refused.
func fencedWriterSetup(t *testing.T) (*ftServer, *Writer, Keys, *ftEvalCounter) {
	t.Helper()
	srv := ftStartRedis(t)
	log := &wtLogger{}
	client, err := commonsredis.NewRedisClient(srv.cfg("ftfencew"), log)
	require.NoError(t, err)
	keys, err := NewKeys(client.Prefix(), "ft-key", "")
	require.NoError(t, err)
	ftSeedLease(t, srv, keys)

	counter := &ftEvalCounter{Client: client}
	w, err := NewWriter(counter, keys, wtFingerprint, ftOpts(), log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return srv, w, keys, counter
}

// fencedWriterMoveLease gives the lease to another owner, so every fenced script call is refused with PROXY_FENCED
func fencedWriterMoveLease(t *testing.T, srv *ftServer, keys Keys) {
	t.Helper()
	require.NoError(t, srv.admin.Set(context.Background(), keys.Lease(), "other", 0).Err())
}

// fencedWriterSnapshot returns every key in the test Redis as its DUMP payload, so a refused write shows as no change
func fencedWriterSnapshot(t *testing.T, srv *ftServer) map[string]string {
	t.Helper()
	ctx := context.Background()
	names, err := srv.admin.Keys(ctx, "*").Result()
	require.NoError(t, err)
	out := make(map[string]string, len(names))
	for _, name := range names {
		dump, err := srv.admin.Dump(ctx, name).Result()
		require.NoError(t, err)
		out[name] = dump
	}
	return out
}

// fencedWriterWaitStartupPurge waits until the worker is idle after Start with no Source. With no Source the worker first
// runs the startup expiry purge, which is one PurgeScript call that also sets the purge-last key, and only then drains
// the queue. The purge-last key existing means that call has run, and an empty queue means nothing else is pending.
// Moving the lease before that point would make the purge, not the write under test, the refused call.
func fencedWriterWaitStartupPurge(t *testing.T, srv *ftServer, keys Keys, w *Writer) {
	t.Helper()
	require.Eventually(t, func() bool {
		n, err := srv.admin.Exists(context.Background(), keys.PurgeLast()).Result()
		return err == nil && n == 1
	}, 10*time.Second, 5*time.Millisecond, "startup expiry purge done")
	require.Eventually(t, func() bool { return w.Status().PendingChanges == 0 }, 10*time.Second, 5*time.Millisecond, "worker idle before the snapshot")
}

// fencedWriterSeedSegment adds one member to each of the three segment zsets (active, tombstones, removal clock), so a
// drop, repair or purge has data to refuse. The meta hash (first key of SegmentKeys) is not touched: it must stay a hash.
func fencedWriterSeedSegment(t *testing.T, srv *ftServer, keys Keys, name string) {
	t.Helper()
	ctx := context.Background()
	for _, k := range keys.SegmentKeys(name)[1:] {
		require.NoError(t, srv.admin.Do(ctx, "ZADD", k, 1, "seeded").Err())
	}
}

// fencedEvals counts the calls of one script on the counter
func fencedEvals(c *ftEvalCounter, script string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.evals[script]
}

// fencedWriterAssertRefused checks the writer state after a fenced refusal on a direct call
func fencedWriterAssertRefused(t *testing.T, w *Writer) {
	t.Helper()
	st := w.Status()
	assert.False(t, st.Healthy, "a fenced writer is not healthy")
	assert.Contains(t, st.LastError, FencedMarker)
}

func TestFencedWriteRejected(t *testing.T) {
	srv, w, keys, counter := fencedWriterSetup(t)
	require.NoError(t, w.Start(nil))
	fencedWriterWaitStartupPurge(t, srv, keys, w)
	fencedWriterMoveLease(t, srv, keys)
	before := fencedWriterSnapshot(t, srv)

	w.PutSegment("seg-a", []string{"seg-a-k1", "seg-a-k2"}, nil, 1)

	require.Eventually(t, func() bool { return !w.Status().Healthy }, 10*time.Second, 20*time.Millisecond, "fenced refusal recorded")
	assert.Equal(t, 1, fencedEvals(counter, SegmentScript), "a fenced segment write is attempted once and not retried")
	assert.Equal(t, before, fencedWriterSnapshot(t, srv), "a refused segment write leaves Redis unchanged")
	fencedWriterAssertRefused(t, w)
}

// fencedWriterSetupChunked is fencedWriterSetup with a small ChunkSize, so one segment write spans several script calls
func fencedWriterSetupChunked(t *testing.T, chunk int) (*ftServer, *Writer, Keys, *ftEvalCounter) {
	t.Helper()
	srv := ftStartRedis(t)
	log := &wtLogger{}
	client, err := commonsredis.NewRedisClient(srv.cfg("ftfencew"), log)
	require.NoError(t, err)
	keys, err := NewKeys(client.Prefix(), "ft-key", "")
	require.NoError(t, err)
	ftSeedLease(t, srv, keys)

	counter := &ftEvalCounter{Client: client}
	opts := ftOpts()
	opts.ChunkSize = chunk
	w, err := NewWriter(counter, keys, wtFingerprint, opts, log)
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return srv, w, keys, counter
}

func TestFencedWriterSegmentWriteMultiChunkRefused(t *testing.T) {
	srv, w, keys, counter := fencedWriterSetupChunked(t, 2)
	require.NoError(t, w.Start(nil))
	fencedWriterWaitStartupPurge(t, srv, keys, w)
	fencedWriterMoveLease(t, srv, keys)
	before := fencedWriterSnapshot(t, srv)

	// five keys with a chunk size of two make three chunks; the lease is already moved, so the first chunk is refused
	w.PutSegment("seg-a", []string{"seg-a-k1", "seg-a-k2", "seg-a-k3", "seg-a-k4", "seg-a-k5"}, nil, 1)

	require.Eventually(t, func() bool { return !w.Status().Healthy }, 10*time.Second, 20*time.Millisecond, "fenced refusal recorded")
	require.Eventually(t, func() bool { return w.Status().PendingChanges == 0 }, 10*time.Second, 5*time.Millisecond, "worker idle after the refusal")
	assert.Equal(t, 1, fencedEvals(counter, SegmentScript), "after the first chunk is refused the remaining chunks are not sent")
	assert.Equal(t, before, fencedWriterSnapshot(t, srv), "a refused multi-chunk segment write leaves Redis unchanged")
	fencedWriterAssertRefused(t, w)
}

func TestFencedWriterSegmentDropRefused(t *testing.T) {
	srv, w, keys, counter := fencedWriterSetup(t)
	fencedWriterSeedSegment(t, srv, keys, "gone")
	fencedWriterMoveLease(t, srv, keys)
	before := fencedWriterSnapshot(t, srv)

	// the segment is in Redis (cursor field) but not listed by the source, so the drop path runs for it
	fields := map[string]string{"seg.gone.till": "1"}
	listed := map[string]struct{}{"kept": {}}
	err := w.dropUnlistedSegments(fields, listed)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFenced)
	assert.Equal(t, 1, fencedEvals(counter, SegmentDropScript), "a fenced segment drop is attempted once and not retried")
	assert.Equal(t, before, fencedWriterSnapshot(t, srv), "a refused segment drop leaves Redis unchanged")
	fencedWriterAssertRefused(t, w)
}

func TestFencedWriterSegmentRepairRefused(t *testing.T) {
	srv, w, keys, counter := fencedWriterSetup(t)
	fencedWriterSeedSegment(t, srv, keys, "gone")
	fencedWriterMoveLease(t, srv, keys)
	before := fencedWriterSnapshot(t, srv)

	err := w.DeleteSegmentKeys("gone")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFenced)
	assert.Equal(t, 1, fencedEvals(counter, SegmentRepairScript), "a fenced segment repair is attempted once and not retried")
	assert.Equal(t, before, fencedWriterSnapshot(t, srv), "a refused segment repair leaves Redis unchanged")
	fencedWriterAssertRefused(t, w)
}

func TestFencedWriterPurgeRefused(t *testing.T) {
	srv, w, keys, counter := fencedWriterSetup(t)
	fencedWriterSeedSegment(t, srv, keys, "gone")
	fencedWriterMoveLease(t, srv, keys)
	before := fencedWriterSnapshot(t, srv)

	err := w.purge(map[string]string{"seg.gone.till": "1"})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFenced)
	assert.Equal(t, 1, fencedEvals(counter, PurgeScript), "a fenced purge is attempted once and not retried")
	assert.Equal(t, before, fencedWriterSnapshot(t, srv), "a refused purge leaves Redis unchanged")
	fencedWriterAssertRefused(t, w)
}

// TestNonHolderPurgeDropRepairFenced is the plan T20 entry point. A non-holder's Drop, Repair and Purge are refused
// without touching Redis, and a release only deletes the lease for the owner at the stored token.
func TestNonHolderPurgeDropRepairFenced(t *testing.T) {
	t.Run("drop", TestFencedWriterSegmentDropRefused)
	t.Run("repair", TestFencedWriterSegmentRepairRefused)
	t.Run("purge", TestFencedWriterPurgeRefused)
	t.Run("release only by owner and token", TestLeaseReleaseOnlyOwnerAndToken)
}
