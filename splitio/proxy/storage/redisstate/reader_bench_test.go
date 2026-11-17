package redisstate

// Needs Redis on localhost:6379 (skips without it, see requireRedis).
// PROXY_BENCH_BIG=1 selects 50 segments x 500k keys; the default is 5 x 20k.
// PROXY_BENCH_SEGMENTS and PROXY_BENCH_KEYS override either size.
// PROXY_BENCH_FLAGS sets the flag count (positive int, default 20). Flag 0 references the segments; the other flags
// carry a whitelist matcher and are about 500 bytes of JSON each.

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
)

// BenchmarkReaderBoot times one Reader.Read over all segments of a state written by the Writer. The write is
// outside the timer.
// This benchmark uses a no-op visitor, so it excludes the restore sink (Bolt transaction, key maps and sets built per
// segment). Peak-heap numbers from here understate the real boot peak. The restore benchmark in the storage package
// (splitio/proxy/storage/redisrestore_test.go) covers the sink.
func BenchmarkReaderBoot(b *testing.B) {
	segments, keysPerSeg := 5, 20000
	if os.Getenv("PROXY_BENCH_BIG") == "1" {
		segments, keysPerSeg = 50, 500000
	}
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_SEGMENTS")); err == nil && n > 0 {
		segments = n
	}
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_KEYS")); err == nil && n > 0 {
		keysPerSeg = n
	}
	b.Run(fmt.Sprintf("segments=%d_keys=%d", segments, keysPerSeg), func(b *testing.B) {
		benchReaderBoot(b, segments, keysPerSeg)
	})
}

func benchReaderBoot(b *testing.B, segments, keysPerSeg int) {
	e := newRealRedisEnv(b)
	segNames := make([]string, segments)
	for s := range segNames {
		segNames[s] = fmt.Sprintf("seg-%03d", s)
	}
	flags := make([]dtos.SplitDTO, 0, benchFlagCount())
	for i := 0; i < cap(flags); i++ {
		flags = append(flags, benchFlagSized(i))
	}
	flags[0].Conditions = []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Matchers: rdMatchers(segNames)}}}
	e.writer.PutFlags(flags, nil, int64(len(flags)))
	for s := 0; s < segments; s++ {
		keys := make([]string, keysPerSeg)
		for k := range keys {
			keys[k] = fmt.Sprintf("s%03d-k%07d", s, k)
		}
		e.writer.PutSegment(segNames[s], keys, nil, 10)
	}
	e.drain(b)

	ctx := context.Background()
	want := segments * keysPerSeg
	var total int
	visit := func(seg SegmentState) error {
		total += len(seg.Active)
		return nil
	}
	warm, err := e.reader.Read(ctx, visit)
	if err != nil {
		b.Fatalf("warm-up read: %v", err)
	}
	if warm.Status != BootValid || total != want {
		b.Fatalf("warm-up read: status %v (%s), %d keys read, want %d", warm.Status, warm.Reason, total, want)
	}

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	peak := base.HeapAlloc
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				runtime.ReadMemStats(&ms)
				if ms.HeapAlloc > peak {
					peak = ms.HeapAlloc
				}
			}
		}
	}()

	rssBase := peakRSSBytes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total = 0
		state, err := e.reader.Read(ctx, visit)
		if err != nil {
			b.Fatalf("boot read: %v", err)
		}
		if state.Status != BootValid || total != want {
			b.Fatalf("boot read: status %v (%s), %d keys read, want %d", state.Status, state.Reason, total, want)
		}
	}
	b.StopTimer()
	close(stop)
	<-done

	var delta float64
	if peak > base.HeapAlloc {
		delta = float64(peak - base.HeapAlloc)
	}
	b.ReportMetric(delta, "peak-heap-B")
	// Peak RSS is the process-lifetime high-water mark, so the delta only shows growth past the setup phase.
	b.ReportMetric(float64(peakRSSBytes()-rssBase), "peak-rss-delta-B")
	b.ReportMetric(float64(peakRSSBytes()), "peak-rss-B")
}

// peakRSSBytes returns the process peak resident set size. ru_maxrss is bytes on darwin and kilobytes on linux.
func peakRSSBytes() uint64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return uint64(ru.Maxrss)
	}
	return uint64(ru.Maxrss) * 1024
}

// benchFlagCount returns PROXY_BENCH_FLAGS when it is a positive int, otherwise 20.
func benchFlagCount() int {
	if n, err := strconv.Atoi(os.Getenv("PROXY_BENCH_FLAGS")); err == nil && n > 0 {
		return n
	}
	return 20
}

// benchFlagSized builds flag i. Flag 0 gets its conditions from the caller (segment references). Other flags get a
// whitelist matcher with 12 user keys so the serialized flag is about 500 bytes.
func benchFlagSized(i int) dtos.SplitDTO {
	d := dtos.SplitDTO{Name: fmt.Sprintf("flag-%06d", i), Status: "ACTIVE", ChangeNumber: int64(i + 1)}
	if i == 0 {
		return d
	}
	users := make([]string, 12)
	for u := range users {
		users[u] = fmt.Sprintf("user-%04d-%010d", i%10000, u)
	}
	d.Conditions = []dtos.ConditionDTO{{MatcherGroup: dtos.MatcherGroupDTO{Combiner: "AND", Matchers: []dtos.MatcherDTO{{
		MatcherType: "WHITELIST",
		Whitelist:   &dtos.WhitelistMatcherDataDTO{Whitelist: users},
	}}}}}
	return d
}
