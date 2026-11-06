package redisstate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// benchRedisAddr returns the Redis address for benchmarks; override with REDISSTATE_BENCH_REDIS
func benchRedisAddr() string {
	if v := os.Getenv("REDISSTATE_BENCH_REDIS"); v != "" {
		return v
	}
	return "localhost:6379"
}

const (
	benchNamespace  = "bench-21980"
	benchWaitLimit  = 10 * time.Minute
	benchSmallKeys  = 10
	benchBigPrefix  = "big-"
	benchSmallPrefx = "seg-"
)

type benchProfile struct {
	flags, segments, bigSegments, keysPerBig int
}

var (
	benchSmallProfile   = benchProfile{flags: 20, segments: 5, bigSegments: 1, keysPerBig: 1000}
	benchExampleProfile = benchProfile{flags: 2000, segments: 150, bigSegments: 4, keysPerBig: 500000}
)

// selectedBenchProfile returns the reduced example profile only when REDISSTATE_BENCH_PROFILE=example
func selectedBenchProfile() benchProfile {
	if os.Getenv("REDISSTATE_BENCH_PROFILE") == "example" {
		return benchExampleProfile
	}
	return benchSmallProfile
}

// benchSource is an in-memory Source built by generateProfile
type benchSource struct {
	flags   []dtos.SplitDTO
	flagsCn int64
	segs    []SegmentState
}

func (s *benchSource) Flags() ([]dtos.SplitDTO, int64, error) { return s.flags, s.flagsCn, nil }

func (s *benchSource) RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) {
	return nil, 0, nil
}

func (s *benchSource) Segments(visit func(SegmentState) error) error {
	for _, seg := range s.segs {
		if err := visit(seg); err != nil {
			return err
		}
	}
	return nil
}

// segmentKeyCount is the number of keys (active plus removed) across all segments
func (s *benchSource) segmentKeyCount() int {
	total := 0
	for _, seg := range s.segs {
		total += len(seg.Active) + len(seg.Removed)
	}
	return total
}

// generateProfile builds a Source with flags flags and segments segments; the first bigSegments segments hold
// keysPerBig active keys each, the others hold benchSmallKeys keys each.
func generateProfile(flags, segments, bigSegments, keysPerBig int) *benchSource {
	if bigSegments > segments {
		panic("generateProfile: bigSegments exceeds segments")
	}
	src := &benchSource{flagsCn: int64(flags)}
	for i := 0; i < flags; i++ {
		src.flags = append(src.flags, dtos.SplitDTO{Name: fmt.Sprintf("flag-%05d", i), ChangeNumber: int64(i + 1), Status: "ACTIVE"})
	}
	for i := 0; i < segments; i++ {
		seg := SegmentState{Removed: map[string]int64{}, ChangeNumber: int64(i + 1)}
		if i < bigSegments {
			seg.Name = fmt.Sprintf("%s%02d", benchBigPrefix, i)
			seg.Active = make(map[string]int64, keysPerBig)
			for k := 0; k < keysPerBig; k++ {
				seg.Active[fmt.Sprintf("b%02d-%07d", i, k)] = int64(i + 1)
			}
		} else {
			seg.Name = fmt.Sprintf("%s%03d", benchSmallPrefx, i)
			seg.Active = make(map[string]int64, benchSmallKeys)
			for k := 0; k < benchSmallKeys; k++ {
				seg.Active[fmt.Sprintf("s%03d-%02d", i, k)] = int64(i + 1)
			}
		}
		src.segs = append(src.segs, seg)
	}
	return src
}

type benchLogger struct{ tb testing.TB }

func (l benchLogger) Error(msg ...interface{})   { l.tb.Log("ERROR", fmt.Sprint(msg...)) }
func (l benchLogger) Warning(msg ...interface{}) {}
func (l benchLogger) Info(msg ...interface{})    {}
func (l benchLogger) Debug(msg ...interface{})   {}
func (l benchLogger) Verbose(msg ...interface{}) {}

type benchEnv struct {
	w    *Writer
	meta func() (map[string]string, error)
	keys Keys
}

// newBenchEnv connects to the local Redis, clears the bench namespace and builds a Writer (not started)
func newBenchEnv(b testing.TB) benchEnv {
	raw, err := redis.NewClient(&redis.UniversalOptions{Addrs: []string{benchRedisAddr()}})
	if err != nil {
		b.Fatalf("redis client: %v", err)
	}
	if err := raw.Ping().Err(); err != nil {
		b.Fatalf("redis at %s not reachable: %v", benchRedisAddr(), err)
	}
	client, err := redis.NewPrefixedRedisClient(raw, "")
	if err != nil {
		b.Fatalf("prefixed client: %v", err)
	}
	keys, err := NewKeys(client.Prefix(), "", benchNamespace)
	if err != nil {
		b.Fatalf("keys: %v", err)
	}
	w, err := NewWriter(client, keys, "bench-fingerprint", DefaultOptions(), benchLogger{tb: b})
	if err != nil {
		b.Fatalf("writer: %v", err)
	}
	if err := w.Reset(); err != nil {
		b.Fatalf("reset: %v", err)
	}
	return benchEnv{
		w:    w,
		meta: func() (map[string]string, error) { return client.HGetAll(keys.MetaUnprefixed()) },
		keys: keys,
	}
}

// waitDrained blocks until the writer has no queued or in-flight change
func waitDrained(b *testing.B, w *Writer) {
	deadline := time.Now().Add(benchWaitLimit)
	for w.Status().PendingChanges > 0 {
		if time.Now().After(deadline) {
			b.Fatalf("writer did not drain, status: %+v", w.Status())
		}
		time.Sleep(10 * time.Microsecond)
	}
}

// waitSegmentCursor blocks until the meta hash holds cursor cn for segment name
func waitSegmentCursor(b *testing.B, env benchEnv, name string, cn int64) {
	deadline := time.Now().Add(benchWaitLimit)
	field := env.keys.SegmentCursorField(name)
	for {
		fields, err := env.meta()
		if err == nil && fields[field] == strconv.FormatInt(cn, 10) {
			return
		}
		if time.Now().After(deadline) {
			b.Fatalf("resync did not finish, last error: %v, status: %+v", err, env.w.Status())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func benchFlag(cn int64) dtos.SplitDTO {
	return dtos.SplitDTO{Name: "flag-bench", ChangeNumber: cn, Status: "ACTIVE"}
}

func benchKeys(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("bk-%07d", i)
	}
	return out
}

func BenchmarkWritePutFlag(b *testing.B) {
	env := newBenchEnv(b)
	env.w.Start(nil)
	b.Cleanup(env.w.Stop)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env.w.PutFlags([]dtos.SplitDTO{benchFlag(int64(i + 1))}, nil, int64(i+1))
		waitDrained(b, env.w)
	}
}

func BenchmarkWriteSegmentChunk(b *testing.B) {
	for _, tc := range []struct {
		name string
		keys int
	}{{"1key", 1}, {"1000keys", 1000}} {
		b.Run(tc.name, func(b *testing.B) {
			env := newBenchEnv(b)
			env.w.Start(nil)
			b.Cleanup(env.w.Stop)
			adds := benchKeys(tc.keys)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				env.w.PutSegment("bench-seg", adds, nil, int64(i+1))
				waitDrained(b, env.w)
			}
		})
	}
}

func BenchmarkResyncExampleProfile(b *testing.B) {
	p := selectedBenchProfile()
	src := generateProfile(p.flags, p.segments, p.bigSegments, p.keysPerBig)
	last := src.segs[len(src.segs)-1]
	totalKeys := src.segmentKeyCount()
	b.Logf("profile flags=%d segments=%d bigSegments=%d keysPerBig=%d segmentKeys=%d", p.flags, p.segments, p.bigSegments, p.keysPerBig, totalKeys)

	var elapsed time.Duration
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		env := newBenchEnv(b)
		b.StartTimer()
		start := time.Now()
		env.w.Start(src)
		waitSegmentCursor(b, env, last.Name, last.ChangeNumber)
		elapsed += time.Since(start)
		b.StopTimer()
		env.w.Stop()
		b.StartTimer()
	}
	b.ReportMetric(elapsed.Seconds()/float64(b.N), "resync_s")
	b.ReportMetric(float64(totalKeys)*float64(b.N)/elapsed.Seconds(), "keys/s")
}

func TestBenchProfileGenerator(t *testing.T) {
	const flags, segments, bigSegments, keysPerBig = 25, 12, 3, 4000
	src := generateProfile(flags, segments, bigSegments, keysPerBig)

	flagList, _, err := src.Flags()
	require.NoError(t, err)
	assert.Len(t, flagList, flags, "flag count")

	var segCount, bigCount, bigKeys int
	require.NoError(t, src.Segments(func(s SegmentState) error {
		segCount++
		if strings.HasPrefix(s.Name, benchBigPrefix) {
			bigCount++
			bigKeys += len(s.Active) + len(s.Removed)
		}
		return nil
	}))
	assert.Equal(t, segments, segCount, "segment count")
	assert.Equal(t, bigSegments, bigCount, "big segment count")
	assert.Equal(t, bigSegments*keysPerBig, bigKeys, "total keys of big segments")
}
