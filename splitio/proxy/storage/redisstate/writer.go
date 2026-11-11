package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/backoff"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
)

const (
	statusActive   = "ACTIVE"
	statusArchived = "ARCHIVED"

	defaultRetention         = 168 * time.Hour
	defaultMaxAttempts       = 3
	defaultBackoffMaxWait    = 60 * time.Second
	defaultMaxPendingChanges = 3000000
	defaultChunkSize         = 1000
	defaultPurgeBatch        = 500
	defaultStopTimeoutMs     = 10000
	// Upper bounds (D25 REV 19c): a Lua script of unbounded size blocks Redis past lua-time-limit.
	MaxChunkSize     = 10000
	MaxPurgeBatch    = 5000
	MaxStopTimeoutMs = 60000

	// noCursor is the cursor argument that makes a script leave the stored cursor untouched
	noCursor = int64(-1)
)

var (
	// ErrEnvironmentMismatch is returned when Redis holds state of another environment/configuration
	ErrEnvironmentMismatch = errors.New("redis state belongs to a different environment or configuration")
	// ErrInvalidOptions is returned when the writer is built with an invalid option or dependency
	ErrInvalidOptions = errors.New("invalid redis state writer options")
	// ErrInvalidSegmentName is returned by DeleteSegmentKeys for an empty name or one with glob characters
	ErrInvalidSegmentName = errors.New("redis state segment name is empty or contains glob characters")

	errStopped       = errors.New("redis state writer stopped")
	errChunkRejected = errors.New("redis state chunk rejected by the script")
	// errDropped is returned once transient retries ran out: the change is not in Redis, which is behind
	errDropped = errors.New("redis state change dropped after transient errors, Redis is behind")
)

// errClass is the D28 class of a Redis error
type errClass int

const (
	errTransient errClass = iota
	errEnvMismatch
	errPermanent
	errUnknown
)

// D28 marker lists. A marker counts only as the first word of the error text, after an optional "ERR " prefix and the
// Lua wrapper (see errHead), so a marker echoed inside an entity name does not change the class.
var (
	transientHeads = []string{"READONLY", "LOADING", "OOM", "TRYAGAIN", "CLUSTERDOWN", "MASTERDOWN", "BUSY", "MOVED", "ASK", "NOREPLICAS", "EXECABORT", "NOAUTH", "PROXY_META_WRONGTYPE"}
	// transientTexts are matched by their exact full text (REV 19c, D39: the go-redis texts include the "redis: " prefix)
	transientTexts = []string{"ERR max number of clients reached", "redis: connection pool timeout", "redis: client is closed"}
	permanentHeads = []string{"PROXY_BAD_ARGS", "WRONGTYPE", "CROSSSLOT"}
)

// classify maps a Redis or client error to its D28 class. The order is the table: transient first (even inside a Lua
// wrapper), then the environment mismatch marker, then permanent (a permanent marker or a Lua runtime error), and
// anything else is unknown.
func classify(err error) errClass {
	msg := err.Error()
	head := errHead(msg)
	var netErr net.Error
	switch {
	case slices.Contains(transientHeads, head), slices.Contains(transientTexts, msg), errors.As(err, &netErr),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, syscall.EPIPE):
		return errTransient
	case ErrorHasMarker(err, EnvMismatchMarker):
		return errEnvMismatch
	case strings.Contains(msg, "@user_script:"), slices.Contains(permanentHeads, head):
		return errPermanent
	}
	return errUnknown
}

// errHead returns the first word of a Redis error after the optional "ERR " prefix and any Lua wrapper
// ("Error running script (...): @user_script:N: ..." or "user_script:N: ...")
func errHead(msg string) string {
	body := strings.TrimPrefix(msg, "ERR ")
	if rest, ok := strings.CutPrefix(body, "Error running script"); ok {
		if _, after, found := strings.Cut(rest, ": "); found {
			body = after
		}
	}
	if rest, ok := strings.CutPrefix(strings.TrimPrefix(body, "@"), "user_script:"); ok {
		if _, after, found := strings.Cut(rest, ": "); found {
			body = after
		}
	}
	head, _, _ := strings.Cut(body, " ")
	return head
}

// Client is the subset of the go-toolkit prefixed Redis client used by the Writer
type Client interface {
	Eval(script string, keys []string, args ...interface{}) error
	// HGetAll reads a hash. The prefixed client adds Prefix()+"." itself, so pass Keys.MetaUnprefixed()
	HGetAll(key string) (map[string]string, error)
	Prefix() string
	// Del is used by Reset. The prefixed client adds Prefix()+"." to the keys itself
	Del(keys ...string) (int64, error)
	// Get reads a string key (the purge count). The prefixed client adds the prefix itself, so pass Keys.Unprefixed().PurgeLast()
	Get(key string) (string, error)
}

var _ Client = (*redis.PrefixedRedisClient)(nil)

// FlagWriter receives feature flag changes
type FlagWriter interface {
	PutFlags(toAdd, toRemove []dtos.SplitDTO, cn int64)
}

// RuleBasedSegmentWriter receives rule-based segment changes
type RuleBasedSegmentWriter interface {
	PutRuleBasedSegments(toAdd, toRemove []dtos.RuleBasedSegmentDTO, cn int64)
}

// SegmentWriter receives segment key changes
type SegmentWriter interface {
	PutSegment(name string, toAdd, toRemove []string, cn int64)
}

// StatusReporter exposes the state of the writer
type StatusReporter interface {
	Status() Status
}

var (
	_ FlagWriter             = (*Writer)(nil)
	_ RuleBasedSegmentWriter = (*Writer)(nil)
	_ SegmentWriter          = (*Writer)(nil)
	_ StatusReporter         = (*Writer)(nil)
)

// Status is a point-in-time view of the writer; the JSON tags are what /observability serves under "redisState".
// Resyncs counts overflow events (every time queued changes were dropped), not the startup resync.
type Status struct {
	Healthy             bool   `json:"healthy"`
	EnvironmentMismatch bool   `json:"environmentMismatch"`
	PendingChanges      int    `json:"pendingChanges"`
	LagMs               int64  `json:"lagMs"`
	LastSuccessUnixMs   int64  `json:"lastSuccessUnixMs"`
	LastError           string `json:"lastError"`
	FailedRounds        int64  `json:"failedRounds"`
	Resyncs             int64  `json:"resyncs"`
	RefusedChanges      int64  `json:"refusedChanges"`
	RejectedChanges     int64  `json:"rejectedChanges"`
	DroppedChanges      int64  `json:"droppedChanges"`
}

// Options configures a Writer
type Options struct {
	Retention         time.Duration // how long removals are kept, default 168h
	MaxAttempts       int           // attempts per script call before the change is dropped, default 3
	BackoffMaxWait    time.Duration // default 60s
	MaxPendingChanges int           // default 3000000 (entities + keys queued; a queued key is 24 bytes, about 72 MB at the cap)
	ChunkSize         int           // default 1000 (entities or keys per script call)
	PurgeBatch        int           // default 500 (entries removed per PurgeScript call), 1..5000
	StopTimeoutMs     int64         // default 10000 (drain bound on Stop, D26), 1..60000
}

// DefaultOptions returns the default writer options
func DefaultOptions() Options {
	return Options{
		Retention:         defaultRetention,
		MaxAttempts:       defaultMaxAttempts,
		BackoffMaxWait:    defaultBackoffMaxWait,
		MaxPendingChanges: defaultMaxPendingChanges,
		ChunkSize:         defaultChunkSize,
		PurgeBatch:        defaultPurgeBatch,
		StopTimeoutMs:     defaultStopTimeoutMs,
	}
}

type changeKind int

const (
	kindFlags changeKind = iota
	kindRuleBasedSegments
	kindSegment
)

type segmentKey struct {
	key string
	cn  int64
}

type change struct {
	kind       changeKind
	enqueuedAt time.Time
	size       int
	cn         int64
	flags      []dtos.SplitDTO
	rbs        []dtos.RuleBasedSegmentDTO
	removals   []bool // true for entities from the removal list of a PutFlags or PutRuleBasedSegments call (D24)
	segment    string
	adds       []segmentKey
	rems       []segmentKey
	// namePos (flags, rule-based segments) and keyPos (segments) map an entity name or key to its position. Built
	// lazily on the first merge (D31), then updated on every merge, so a merge costs O(size of the merged change).
	namePos map[string]int
	keyPos  map[string]keyPos
}

// keyPos locates a segment key in change.adds (add true) or change.rems.
type keyPos struct {
	add bool
	idx int
}

// groupKey identifies the queue group of a change: flags, rule-based segments, or one segment name (D19, D31).
type groupKey struct {
	kind    changeKind
	segment string
}

func groupOf(c *change) groupKey {
	if c.kind == kindSegment {
		return groupKey{kind: kindSegment, segment: c.segment}
	}
	return groupKey{kind: c.kind}
}

type entity struct {
	name    string
	cn      int64
	removed bool
	json    string
}

type job struct {
	resync bool
	change *change
}

// Writer mirrors the proxy state into Redis. Put* only enqueue; a single worker goroutine talks to Redis.
// Lock order: the facade mutex is taken before mtx. The worker holds mtx only to read or change queue and
// status state, never across Source calls, Eval, HGetAll or a wait.
type Writer struct {
	// overflow and overflowEpoch are guarded by mtx (D21). overflow freezes the cursors of every group until a resync
	// with a Source completes; overflowEpoch counts overflows so a resync can tell whether one happened after it started.
	overflow      bool
	overflowEpoch uint64
	// resyncPass and skipsSeen are owned by the worker goroutine: resyncPass ignores overflow (a resync rewrites the whole
	// state with source cursors), skipsSeen records any skip during the pass.
	resyncPass bool
	skipsSeen  bool

	client      Client
	keys        Keys
	fingerprint string
	opts        Options
	logger      logging.LoggerInterface
	groups      map[string]*groupFreeze // D19 in-memory group state, keyed by cursor field; lazily created by freezeOf

	now     func() time.Time
	wait    func(time.Duration) bool
	marshal func(v interface{}) ([]byte, error)

	signal   chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	// drainUntil is the drain deadline (D26), written by Stop before w.stop closes and read only after the close is observed
	drainUntil time.Time

	mtx        sync.Mutex
	started    bool
	queue      []*change
	queuedSize int
	// groupIdx maps a group to its last queued change (D31). A change leaves the index when it is popped or the queue is cleared.
	groupIdx    map[groupKey]*change
	inflight    *change
	needResync  bool
	canResync   bool
	resyncSince time.Time
	mismatch    bool
	// dead is set by recoverWorker after a worker panic: nothing is queued again, the mirror stays stopped
	dead          bool
	refusedLogged bool
	// stopRefusedLogged is guarded by mtx; woken is owned by the worker goroutine (see sleep)
	stopRefusedLogged bool
	woken             bool
	stats             Status
}

// NewWriter validates opts (all > 0, else an error wrapping ErrInvalidOptions) and builds a Writer. It does not touch Redis.
func NewWriter(client Client, keys Keys, fingerprint string, opts Options, logger logging.LoggerInterface) (*Writer, error) {
	switch {
	case client == nil:
		return nil, fmt.Errorf("%w: client is required", ErrInvalidOptions)
	case logger == nil:
		return nil, fmt.Errorf("%w: logger is required", ErrInvalidOptions)
	case fingerprint == "":
		return nil, fmt.Errorf("%w: fingerprint is required", ErrInvalidOptions)
	case opts.Retention <= 0:
		return nil, fmt.Errorf("%w: retention must be positive", ErrInvalidOptions)
	case opts.MaxAttempts <= 0:
		return nil, fmt.Errorf("%w: max attempts must be positive", ErrInvalidOptions)
	case opts.BackoffMaxWait <= 0:
		return nil, fmt.Errorf("%w: backoff max wait must be positive", ErrInvalidOptions)
	case opts.MaxPendingChanges <= 0:
		return nil, fmt.Errorf("%w: max pending changes must be positive", ErrInvalidOptions)
	case opts.ChunkSize <= 0 || opts.ChunkSize > MaxChunkSize:
		return nil, fmt.Errorf("%w: chunk size must be 1..%d", ErrInvalidOptions, MaxChunkSize)
	case opts.PurgeBatch <= 0 || opts.PurgeBatch > MaxPurgeBatch:
		return nil, fmt.Errorf("%w: purge batch must be 1..%d", ErrInvalidOptions, MaxPurgeBatch)
	case opts.StopTimeoutMs <= 0 || opts.StopTimeoutMs > MaxStopTimeoutMs:
		return nil, fmt.Errorf("%w: stop timeout ms must be 1..%d", ErrInvalidOptions, MaxStopTimeoutMs)
	}

	w := &Writer{
		client:      client,
		keys:        keys,
		fingerprint: fingerprint,
		opts:        opts,
		logger:      logger,
		now:         time.Now,
		marshal:     json.Marshal,
		signal:      make(chan struct{}, 1),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		stats:       Status{Healthy: true},
	}
	w.wait = w.sleep
	return w, nil
}

// Start runs the D30 probe, then launches the single background worker. The worker first calls source.Resync (if source != nil), then drains the queue.
// A probe that fails with a script error returns a minimum-version InitError and starts no worker.
func (w *Writer) Start(source Source) error {
	if !hasSource(source) {
		source = nil
	}
	if err := w.probe(); err != nil {
		return err
	}
	w.mtx.Lock()
	if w.started {
		w.mtx.Unlock()
		w.logger.Warning("redis state writer already started")
		return nil
	}
	if w.stopped() {
		// Stop ran first: no worker is launched, so nothing can outlive the Stop that already returned
		w.mtx.Unlock()
		return nil
	}
	w.started = true
	w.canResync = source != nil
	if source != nil {
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	go w.run(source)
	return nil
}

// probe runs startProbeScript once under the namespace (D30). A Redis error reply that is not transient means the
// server can not run TIME-then-write scripts, so it returns a minimum-version InitError. Any other error (transport,
// timeout) is not a version error: it is logged and the worker retries through its normal path.
func (w *Writer) probe() error {
	err := w.client.Eval(startProbeScript, []string{w.keys.Unprefixed().base + ".probe"})
	if err == nil {
		return nil
	}
	var reply interface{ RedisError() }
	if errors.As(err, &reply) && classify(err) != errTransient {
		return common.NewInitError(fmt.Errorf("redis state writer requires Redis 5.0 or later (scripts call TIME then write): %w", err), common.ExitRedisInitializationFailed)
	}
	w.logger.Warning("redis state writer start probe failed, the worker will retry: ", err)
	return nil
}

// Stop drains the queue for at most StopTimeoutMs, then drops what is left and returns after the worker exits. New
// input is refused from the call on. Dropped changes never had their cursors written, so Redis stays behind the source.
func (w *Writer) Stop() {
	w.stopOnce.Do(func() {
		w.drainUntil = w.now().Add(time.Duration(w.opts.StopTimeoutMs) * time.Millisecond)
		close(w.stop)
	})
	w.mtx.Lock()
	started := w.started
	w.mtx.Unlock()
	if started {
		<-w.done
	}
	w.mtx.Lock()
	dropped := len(w.queue)
	if w.inflight != nil {
		dropped++
		w.inflight = nil
	}
	w.clearQueueLocked()
	w.mtx.Unlock()
	w.logger.Info(fmt.Sprintf("redis state writer stopped, %d queued changes dropped (drain bound %d ms), their cursors were not written so Redis stays behind the source", dropped, w.opts.StopTimeoutMs))
}

// RequestResync asks the worker to run a full resync of Redis against the Source given to Start, through the same
// path as the startup, refused-chunk and overflow resyncs. Call it once the Source is populated (for example after
// the first successful sync) so state skipped on a cold boot can be repaired. It never blocks and is safe to
// call from any goroutine any number of times: calls made while a resync is scheduled but not started coalesce into
// one. It does nothing before Start, when Start got no Source, after Stop, or after an environment mismatch. A
// requested resync is not counted in Status.Resyncs.
func (w *Writer) RequestResync() {
	w.mtx.Lock()
	ok := w.canResync && !w.mismatch && !w.stopped()
	if ok {
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	if ok {
		w.wake()
	}
}

// Reset deletes every key of this environment namespace it knows of and clears the mismatch flag. Call before Start, only when Initialization.ForceFreshStartup is set.
func (w *Writer) Reset() error {
	if err := w.deleteKnown(); err != nil {
		return fmt.Errorf("deleting the redis state of environment %s: %w", w.keys.Env(), err)
	}
	w.mtx.Lock()
	w.clearQueueLocked()
	w.mismatch = false
	w.refusedLogged = false
	w.stats.Healthy = true
	w.stats.LastError = ""
	w.stats.FailedRounds = 0
	w.mtx.Unlock()
	return nil
}

// deleteKnown deletes the keys it can name without SCAN or KEYS: the flag and rule-based segment keys, and for each
// segment listed in the meta hash (including cursor -1) its three keys. The meta hash is deleted last, because the
// segment names are only known from it: if a DEL fails, meta survives and a rerun finds the rest.
func (w *Writer) deleteKnown() error {
	u := w.keys.Unprefixed()
	meta, err := w.client.HGetAll(u.Meta())
	if err != nil {
		return err
	}
	known := []string{
		u.PurgeLast(), u.FlagData(), u.FlagIndex(), u.FlagRemoved(),
		u.RuleBasedSegmentData(), u.RuleBasedSegmentIndex(), u.RuleBasedSegmentRemoved(),
	}
	for field := range meta {
		if name, ok := segmentNameOfCursorField(field); ok {
			known = append(known, u.SegmentActive(name), u.SegmentTombstones(name), u.SegmentRemovalClock(name))
		}
	}
	for _, key := range known {
		if _, err := w.client.Del(key); err != nil {
			return err
		}
	}
	_, err = w.client.Del(u.Meta())
	return err
}

// DeleteSegmentKeys deletes the active, tombstone and removal clock keys of one segment and no other key, meta included.
// The segment write that follows rewrites the cursor (D38). It uses the worker's retry state, so call it before Start or from the worker (resync) only.
func (w *Writer) DeleteSegmentKeys(name string) error {
	if !validSegmentName(name) {
		return ErrInvalidSegmentName
	}
	u := w.keys.Unprefixed()
	keys := []string{u.SegmentActive(name), u.SegmentTombstones(name), u.SegmentRemovalClock(name)}
	return w.retry("segment key delete", func() error {
		_, err := w.client.Del(keys...)
		return err
	})
}

// segmentNameOfCursorField returns the segment name of a meta field "seg.<name>.till"
func segmentNameOfCursorField(field string) (string, bool) {
	const prefix, suffix = "seg.", ".till"
	if len(field) <= len(prefix)+len(suffix) || !strings.HasPrefix(field, prefix) || !strings.HasSuffix(field, suffix) {
		return "", false
	}
	return field[len(prefix) : len(field)-len(suffix)], true
}

// PutFlags enqueues a feature flag change. It never blocks and never calls Redis.
func (w *Writer) PutFlags(toAdd, toRemove []dtos.SplitDTO, cn int64) {
	if w.refuse() {
		return
	}
	list := make([]dtos.SplitDTO, 0, len(toAdd)+len(toRemove))
	list = append(append(list, toAdd...), toRemove...)
	removals := make([]bool, len(list))
	for i := len(toAdd); i < len(list); i++ {
		removals[i] = true
	}
	w.enqueue(&change{kind: kindFlags, cn: cn, flags: list, removals: removals, size: len(list)})
}

// PutRuleBasedSegments enqueues a rule-based segment change. It never blocks and never calls Redis.
func (w *Writer) PutRuleBasedSegments(toAdd, toRemove []dtos.RuleBasedSegmentDTO, cn int64) {
	if w.refuse() {
		return
	}
	list := make([]dtos.RuleBasedSegmentDTO, 0, len(toAdd)+len(toRemove))
	list = append(append(list, toAdd...), toRemove...)
	removals := make([]bool, len(list))
	for i := len(toAdd); i < len(list); i++ {
		removals[i] = true
	}
	w.enqueue(&change{kind: kindRuleBasedSegments, cn: cn, rbs: list, removals: removals, size: len(list)})
}

// PutSegment enqueues a segment key change; every key gets the change number cn. It never blocks and never calls Redis.
func (w *Writer) PutSegment(name string, toAdd, toRemove []string, cn int64) {
	if !validSegmentName(name) {
		w.reject(fmt.Sprintf("redis state: segment name %q contains glob characters or is empty, change skipped", name))
		return
	}
	if w.refuse() {
		return
	}
	adds := make([]segmentKey, len(toAdd))
	for i, k := range toAdd {
		adds[i] = segmentKey{key: k, cn: cn}
	}
	rems := make([]segmentKey, len(toRemove))
	for i, k := range toRemove {
		rems[i] = segmentKey{key: k, cn: cn}
	}
	w.enqueue(&change{kind: kindSegment, cn: cn, segment: name, adds: adds, rems: rems, size: len(adds) + len(rems)})
}

// Status returns a point-in-time view of the writer.
func (w *Writer) Status() Status {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	s := w.stats
	s.EnvironmentMismatch = w.mismatch
	s.PendingChanges = len(w.queue)
	if w.inflight != nil {
		s.PendingChanges++
	}

	now := w.now()
	var lag time.Duration
	if w.inflight != nil {
		lag = now.Sub(w.inflight.enqueuedAt)
	} else if len(w.queue) > 0 {
		lag = now.Sub(w.queue[0].enqueuedAt)
	}
	if !w.resyncSince.IsZero() {
		lag = max(lag, now.Sub(w.resyncSince))
	}
	s.LagMs = max(lag.Milliseconds(), 0)
	return s
}

// hasSource reports whether s holds a usable Source. A nil interface and an interface holding a nil pointer both count as no Source.
func hasSource(s Source) bool {
	if s == nil {
		return false
	}
	v := reflect.ValueOf(s)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface, reflect.Chan:
		return !v.IsNil()
	}
	return true
}

func validSegmentName(name string) bool {
	return name != "" && !strings.ContainsAny(name, globChars)
}

// refuse counts and reports a change refused because of an environment mismatch
func (w *Writer) refuse() bool {
	w.mtx.Lock()
	if !w.mismatch {
		w.mtx.Unlock()
		return false
	}
	w.stats.RefusedChanges++
	first := !w.refusedLogged
	w.refusedLogged = true
	w.mtx.Unlock()
	if first {
		w.logger.Error("redis state: change refused because Redis holds the state of another environment or configuration; further refusals are logged at debug level")
	} else {
		w.logger.Debug("redis state: change refused (environment mismatch)")
	}
	return true
}

func (w *Writer) reject(msg string) {
	w.mtx.Lock()
	w.stats.RejectedChanges++
	w.mtx.Unlock()
	w.logger.Error(msg)
}

func (w *Writer) enqueue(c *change) {
	c.enqueuedAt = w.now()
	var overflowMsg string

	w.mtx.Lock()
	if w.stopped() {
		// D26 (d): once Stop is called the change is refused, so a busy source cannot extend the drain
		w.stats.DroppedChanges++
		first := !w.stopRefusedLogged
		w.stopRefusedLogged = true
		w.mtx.Unlock()
		if first {
			w.logger.Info("redis state: change refused after Stop, the writer is draining or stopped; further refusals are not logged")
		}
		return
	}
	if w.mismatch || w.dead {
		w.stats.RefusedChanges++
		w.mtx.Unlock()
		return
	}
	if len(w.queue) > 0 && w.sizeAfterLocked(c) > w.opts.MaxPendingChanges {
		dropped := len(w.queue)
		w.clearQueueLocked()
		w.overflow = true
		w.overflowEpoch++
		if w.canResync {
			w.stats.Resyncs++
		}
		w.scheduleResyncLocked()
		outcome := "a full resync from the local state is scheduled"
		if !w.canResync {
			outcome = "no resync is scheduled because no source is configured"
		}
		overflowMsg = fmt.Sprintf("redis state: more than %d changes pending, %d queued changes dropped and %s", w.opts.MaxPendingChanges, dropped, outcome)
	}
	if target := w.groupIdx[groupOf(c)]; target != nil {
		w.mergeLocked(target, c)
		w.mtx.Unlock()
		w.logOverflow(overflowMsg)
		w.wake()
		return
	}
	if w.groupIdx == nil {
		w.groupIdx = make(map[groupKey]*change)
	}
	w.groupIdx[groupOf(c)] = c
	w.queue = append(w.queue, c)
	w.queuedSize += c.size
	w.mtx.Unlock()
	w.logOverflow(overflowMsg)
	w.wake()
}

// logOverflow logs an overflow decided under mtx. It runs after the unlock, so the logger is never called under the lock (G-12).
func (w *Writer) logOverflow(msg string) {
	if msg != "" {
		w.logger.Error(msg)
	}
}

// mergeLocked folds c into the last queued change q of the same group (D31). q is never the in-flight change:
// a popped change leaves the group index first. The caller holds mtx.
func (w *Writer) mergeLocked(q, c *change) {
	before := q.size
	switch q.kind {
	case kindFlags:
		q.mergeFlags(c)
	case kindRuleBasedSegments:
		q.mergeRuleBased(c)
	case kindSegment:
		q.mergeKeys(c)
	}
	if c.enqueuedAt.Before(q.enqueuedAt) {
		q.enqueuedAt = c.enqueuedAt
	}
	if c.cn > q.cn {
		q.cn = c.cn
	}
	w.queuedSize += q.size - before
}

// clearQueueLocked empties the queue and the group index. The caller holds mtx. The in-flight change is not touched.
func (w *Writer) clearQueueLocked() {
	w.queue = nil
	w.queuedSize = 0
	w.groupIdx = nil
}

// sizeAfterLocked returns queuedSize as it would be once c is queued: c merged into its group's queued change, or
// appended as a new change. The queue contents are not changed (D31 rule 6, D21). The caller holds mtx.
func (w *Writer) sizeAfterLocked(c *change) int {
	target := w.groupIdx[groupOf(c)]
	if target == nil {
		return w.queuedSize + c.size
	}
	return w.queuedSize - target.size + target.mergedSize(c)
}

// mergedSize returns the size q would have after merging c. Only entities of c that q does not hold yet add to it.
// The name or key index of q is built here if missing, as the merge would build it; the queued entries are not changed.
func (q *change) mergedSize(c *change) int {
	fresh := 0
	seen := make(map[string]struct{})
	switch q.kind {
	case kindFlags:
		q.indexNames()
		for i := range c.flags {
			if _, ok := q.namePos[c.flags[i].Name]; !ok && !seenName(seen, c.flags[i].Name) {
				fresh++
			}
		}
		return len(q.flags) + fresh
	case kindRuleBasedSegments:
		q.indexNames()
		for i := range c.rbs {
			if _, ok := q.namePos[c.rbs[i].Name]; !ok && !seenName(seen, c.rbs[i].Name) {
				fresh++
			}
		}
		return len(q.rbs) + fresh
	}
	q.indexKeys()
	for _, k := range c.rems {
		if _, ok := q.keyPos[k.key]; !ok && !seenName(seen, k.key) {
			fresh++
		}
	}
	for _, k := range c.adds {
		if _, ok := q.keyPos[k.key]; !ok && !seenName(seen, k.key) {
			fresh++
		}
	}
	return len(q.adds) + len(q.rems) + fresh
}

// seenName reports whether name was already counted, and records it otherwise.
func seenName(seen map[string]struct{}, name string) bool {
	if _, ok := seen[name]; ok {
		return true
	}
	seen[name] = struct{}{}
	return false
}

// indexNames builds the name-to-position map of a flags or rule-based segments change once (D31 rev 21).
func (q *change) indexNames() {
	if q.namePos != nil {
		return
	}
	q.namePos = make(map[string]int)
	for i := range q.flags {
		q.namePos[q.flags[i].Name] = i
	}
	for i := range q.rbs {
		q.namePos[q.rbs[i].Name] = i
	}
}

// indexKeys builds the key-to-position map of a segment change once (D31 rev 21).
func (q *change) indexKeys() {
	if q.keyPos != nil {
		return
	}
	q.keyPos = make(map[string]keyPos, len(q.adds)+len(q.rems))
	for i := range q.rems {
		q.keyPos[q.rems[i].key] = keyPos{add: false, idx: i}
	}
	for i := range q.adds {
		q.keyPos[q.adds[i].key] = keyPos{add: true, idx: i}
	}
}

// mergeFlags keeps, per flag name, the entry with the higher change number; at an equal cn the later arrival wins.
func (q *change) mergeFlags(c *change) {
	q.indexNames()
	for i := range c.flags {
		d := c.flags[i]
		if p, ok := q.namePos[d.Name]; ok {
			if d.ChangeNumber >= q.flags[p].ChangeNumber {
				q.flags[p] = d
				q.removals[p] = c.removals[i]
			}
			continue
		}
		q.namePos[d.Name] = len(q.flags)
		q.flags = append(q.flags, d)
		q.removals = append(q.removals, c.removals[i])
	}
	q.size = len(q.flags)
}

// mergeRuleBased keeps, per rule-based segment name, the entry with the higher change number; at an equal cn the later arrival wins.
func (q *change) mergeRuleBased(c *change) {
	q.indexNames()
	for i := range c.rbs {
		d := c.rbs[i]
		if p, ok := q.namePos[d.Name]; ok {
			if d.ChangeNumber >= q.rbs[p].ChangeNumber {
				q.rbs[p] = d
				q.removals[p] = c.removals[i]
			}
			continue
		}
		q.namePos[d.Name] = len(q.rbs)
		q.rbs = append(q.rbs, d)
		q.removals = append(q.removals, c.removals[i])
	}
	q.size = len(q.rbs)
}

// mergeKeys merges a segment change into q (same segment name). Removals are applied before adds of c, so an add and
// a remove at the same cn leave the key added (D28, S-1). Per key the higher cn wins.
func (q *change) mergeKeys(c *change) {
	q.indexKeys()
	for _, k := range c.rems {
		q.putKey(k, false)
	}
	for _, k := range c.adds {
		q.putKey(k, true)
	}
	q.size = len(q.adds) + len(q.rems)
}

// putKey stores k in the add or removal list of q, replacing an entry with a lower cn, or with an equal cn when it is a removal and k is an add.
func (q *change) putKey(k segmentKey, add bool) {
	p, ok := q.keyPos[k.key]
	if !ok {
		q.insertKey(k, add)
		return
	}
	var cur segmentKey
	if p.add {
		cur = q.adds[p.idx]
	} else {
		cur = q.rems[p.idx]
	}
	if k.cn < cur.cn || (k.cn == cur.cn && (p.add || !add)) {
		return
	}
	if p.add == add {
		if add {
			q.adds[p.idx] = k
		} else {
			q.rems[p.idx] = k
		}
		return
	}
	q.removeKey(p)
	q.insertKey(k, add)
}

func (q *change) insertKey(k segmentKey, add bool) {
	if add {
		q.adds = append(q.adds, k)
		q.keyPos[k.key] = keyPos{add: true, idx: len(q.adds) - 1}
		return
	}
	q.rems = append(q.rems, k)
	q.keyPos[k.key] = keyPos{add: false, idx: len(q.rems) - 1}
}

// removeKey drops the entry at p by moving the last entry of its list into the hole (order inside a change is not kept)
func (q *change) removeKey(p keyPos) {
	if p.add {
		last := len(q.adds) - 1
		if p.idx != last {
			q.adds[p.idx] = q.adds[last]
			q.keyPos[q.adds[p.idx].key] = keyPos{add: true, idx: p.idx}
		}
		q.adds = q.adds[:last]
		return
	}
	last := len(q.rems) - 1
	if p.idx != last {
		q.rems[p.idx] = q.rems[last]
		q.keyPos[q.rems[p.idx].key] = keyPos{add: false, idx: p.idx}
	}
	q.rems = q.rems[:last]
}

func (w *Writer) wake() {
	select {
	case w.signal <- struct{}{}:
	default:
	}
}

// scheduleResyncLocked must be called with mtx held
func (w *Writer) scheduleResyncLocked() {
	if !w.canResync {
		return
	}
	w.needResync = true
	if w.resyncSince.IsZero() {
		w.resyncSince = w.now()
	}
}

// scheduleRefusedResync schedules a full resync after a live chunk was refused with PROXY_BAD_ARGS, so the data of
// that chunk is reconciled from the local state. Only live changes call it: a chunk refused during a resync is
// skipped and never schedules another pass, so a bad entity cannot loop.
func (w *Writer) scheduleRefusedResync() {
	w.mtx.Lock()
	if w.canResync && !w.mismatch {
		w.stats.Resyncs++
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	w.wake()
}

// sleep waits d and reports whether the worker should try again. A Stop wakes a sleeping worker once and one immediate
// attempt follows; every later wait while draining runs min(d, time left to the drain deadline) (D26 b).
// woken is owned by the worker goroutine.
func (w *Writer) sleep(d time.Duration) bool {
	if w.stopped() {
		if !w.woken {
			w.woken = true
			return !w.aborted()
		}
		left := w.drainUntil.Sub(w.now())
		if left <= 0 {
			return false
		}
		if d > left {
			d = left
		}
		t := time.NewTimer(d)
		defer t.Stop()
		<-t.C
		return !w.aborted()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-w.stop:
		w.woken = true
	}
	return !w.aborted()
}

// stopped reports whether Stop was called. The writer may still be draining, see aborted.
func (w *Writer) stopped() bool {
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

// aborted reports whether work must end now: Stop was called and the drain deadline has passed.
func (w *Writer) aborted() bool {
	return w.stopped() && !w.now().Before(w.drainUntil)
}

func (w *Writer) run(source Source) {
	defer close(w.done)
	// registered after close, so it runs first: a panic in the worker, a resync pass or a Source callback (all of them
	// run on this goroutine) is recovered and the worker exits through close(w.done)
	defer w.recoverWorker()
	// the startup purge (D27) runs after the first resync completes, or at once when there is no Source
	purgePending := true
	if source == nil {
		// a purge cut short by Stop is left to the next start; the queue is still drained (D26)
		// the purge logs its own outcome; a Stop cut is left to the next start and a mismatch is already recorded
		_ = w.startupPurge()
		purgePending = false
	}
	// dropBo paces the resync requests while a resync keeps dropping (D36), so an outage is not hammered
	dropBo := backoff.New(2, w.opts.BackoffMaxWait)
	for {
		j, ok := w.next()
		if !ok {
			return
		}
		if j.resync {
			err := w.resync(source)
			w.finishResync()
			if errors.Is(err, errStopped) {
				w.logger.Info("redis state: resync cut short by Stop, it reruns at the next start")
				continue
			}
			if errors.Is(err, errDropped) {
				if w.wait(dropBo.Next()) {
					w.RequestResync()
				}
				continue
			}
			if err != nil && !errors.Is(err, ErrEnvironmentMismatch) {
				w.logger.Error(fmt.Sprintf("redis state: resync failed, it reruns at the next request or start: %s", err.Error()))
			}
			dropBo = backoff.New(2, w.opts.BackoffMaxWait)
			if purgePending && err == nil {
				purgePending = false
				// the purge logs its own outcome; a Stop cut is left to the next start and a mismatch is already recorded
				_ = w.startupPurge()
			}
		} else {
			refused, err := w.write(j.change)
			if errors.Is(err, errStopped) {
				// the change stays inflight so Stop counts it as dropped
				return
			}
			w.finishChange()
			if errors.Is(err, errDropped) {
				// the drop freezes the group; a resync releases it and rewrites the lost change
				w.RequestResync()
			} else if refused {
				w.scheduleRefusedResync()
			}
		}
	}
}

// recoverWorker turns a panic on the worker goroutine into one Error line with a stack and an unhealthy status. The
// payload is not logged: it may hold entity data. The mirror stops; the proxy keeps running.
func (w *Writer) recoverWorker() {
	r := recover()
	if r == nil {
		return
	}
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.LastError = fmt.Sprintf("redis state writer stopped after a panic (%T)", r)
	w.dead = true
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: writer worker panicked, the mirror stops and the proxy keeps running: %T\n%s", r, debug.Stack()))
}

// next blocks until there is something to do. A pending resync goes before queued changes.
func (w *Writer) next() (job, bool) {
	for {
		w.mtx.Lock()
		if w.aborted() {
			w.mtx.Unlock()
			return job{}, false
		}
		if !w.mismatch {
			if w.needResync {
				w.needResync = false
				if w.stopped() {
					// draining: a pending resync is skipped so the queue is served first (D26 c)
					w.resyncSince = time.Time{}
					w.mtx.Unlock()
					w.logger.Info("redis state: pending resync skipped during the shutdown drain, the queued changes are written first")
					continue
				}
				w.mtx.Unlock()
				return job{resync: true}, true
			}
			if len(w.queue) > 0 {
				c := w.queue[0]
				w.queue[0] = nil
				w.queue = w.queue[1:]
				if len(w.queue) == 0 {
					w.queue = nil
				}
				w.queuedSize -= c.size
				if w.groupIdx[groupOf(c)] == c {
					delete(w.groupIdx, groupOf(c))
				}
				w.inflight = c
				w.mtx.Unlock()
				return job{change: c}, true
			}
		}
		w.mtx.Unlock()

		select {
		case <-w.signal:
		case <-w.stop:
			return job{}, false
		}
	}
}

func (w *Writer) finishChange() {
	w.mtx.Lock()
	w.inflight = nil
	w.mtx.Unlock()
}

func (w *Writer) finishResync() {
	w.mtx.Lock()
	if !w.needResync {
		w.resyncSince = time.Time{}
	}
	w.mtx.Unlock()
}

// enterMismatch stops all writing: the queue is cleared and later changes are refused until Reset
func (w *Writer) enterMismatch() {
	w.mtx.Lock()
	first := !w.mismatch
	w.mismatch = true
	dropped := len(w.queue)
	w.clearQueueLocked()
	w.inflight = nil
	w.needResync = false
	w.resyncSince = time.Time{}
	w.stats.Healthy = false
	w.stats.LastError = ErrEnvironmentMismatch.Error()
	w.mtx.Unlock()
	if first {
		w.logger.Error(fmt.Sprintf("redis state: the state in Redis belongs to a different environment or configuration (environment %s); nothing will be written, %d queued changes dropped. Use a different namespace or start with a forced fresh startup", w.keys.Env(), dropped))
	}
}

func (w *Writer) recordSuccess() {
	w.mtx.Lock()
	w.stats.Healthy = true
	w.stats.LastError = ""
	w.stats.FailedRounds = 0
	w.stats.LastSuccessUnixMs = w.now().UnixMilli()
	w.mtx.Unlock()
}

func (w *Writer) recordFailedRound(what string, attempts int, last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.FailedRounds++
	w.stats.LastError = last.Error()
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: %s failed %d times in a row: %s", what, attempts, last.Error()))
}

// recordDrop reports a change dropped after its transient tries ran out: one Error line, unhealthy until a write succeeds
func (w *Writer) recordDrop(what string, attempts int, last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.FailedRounds++
	w.stats.LastError = last.Error()
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: %s dropped after %d attempts, Redis is behind the source until the next resync: %s", what, attempts, last.Error()))
}

// recordRefused marks the writer unhealthy after a permanent rejection. It logs nothing: the caller logs the skipped entity.
func (w *Writer) recordRefused(last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.LastError = last.Error()
	w.mtx.Unlock()
}

// retry runs op up to MaxAttempts times with exponential backoff between tries. It returns early with
// ErrEnvironmentMismatch (no retry), errChunkRejected (bad arguments or an unknown error, no retry: the caller isolates
// the refused entity and reports it), errDropped once the transient tries are exhausted (the caller does not isolate
// entities, the change is lost until a resync) or errStopped once the drain deadline has passed.
func (w *Writer) retry(what string, op func() error) error {
	bo := backoff.New(2, w.opts.BackoffMaxWait)
	var last error
	for attempt := 1; attempt <= w.opts.MaxAttempts; attempt++ {
		// a running resync or purge pass stops at its next Eval once draining starts (D26 c)
		if w.aborted() || w.resyncPass && w.stopped() {
			return errStopped
		}
		err := op()
		if err == nil {
			return nil
		}
		class := classify(err)
		switch class {
		case errEnvMismatch:
			w.enterMismatch()
			return ErrEnvironmentMismatch
		case errPermanent:
			w.recordRefused(err)
			return errChunkRejected
		}
		last = err
		if class == errUnknown && attempt == w.opts.MaxAttempts {
			w.recordFailedRound(what, attempt, last)
			return errChunkRejected
		}
		if attempt < w.opts.MaxAttempts && !w.wait(bo.Next()) {
			return errStopped
		}
	}
	// a change that runs out of tries while Stop drains stays in flight, so Stop counts it as dropped
	if w.stopped() {
		return errStopped
	}
	// D36: the cap ends the retries, so a long outage cannot stall the queue; the resync repairs the dropped change
	w.recordDrop(what, w.opts.MaxAttempts, last)
	return errDropped
}

// eval runs one script call with retry and records the success
func (w *Writer) eval(what, script string, keys []string, args []interface{}) error {
	return w.retry(what, func() error {
		err := w.client.Eval(script, keys, args...)
		if err == nil {
			w.recordSuccess()
		}
		return err
	})
}

func (w *Writer) write(c *change) (bool, error) {
	switch c.kind {
	case kindFlags:
		ents, skipped := w.flagEntities(c.flags, c.removals)
		return w.writeEntities("flag write", FlagScript, w.keys.FlagKeys(), w.keys.FlagCursorField(), ents, c.cn, skipped)
	case kindRuleBasedSegments:
		ents, skipped := w.rbsEntities(c.rbs, c.removals)
		return w.writeEntities("rule-based segment write", RuleBasedSegmentScript, w.keys.RuleBasedSegmentKeys(), w.keys.RuleBasedSegmentCursorField(), ents, c.cn, skipped)
	default:
		return w.writeSegment(c.segment, c.adds, c.rems, c.cn, false)
	}
}

// flagEntities serializes feature flags. The removed flag comes from the DTO status, never from the list it arrived in.
func (w *Writer) flagEntities(list []dtos.SplitDTO, removals []bool) ([]entity, []string) {
	ents := make([]entity, 0, len(list))
	var skipped []string
	for i := range list {
		d := &list[i]
		removed := d.Status != statusActive || (removals != nil && removals[i])
		e, ok := w.newEntity(d.Name, d.ChangeNumber, removed, *d)
		if !ok {
			skipped = append(skipped, d.Name)
			continue
		}
		ents = append(ents, e)
	}
	return ents, skipped
}

// rbsEntities serializes rule-based segments with the same rules as flagEntities
func (w *Writer) rbsEntities(list []dtos.RuleBasedSegmentDTO, removals []bool) ([]entity, []string) {
	ents := make([]entity, 0, len(list))
	var skipped []string
	for i := range list {
		d := &list[i]
		removed := d.Status != statusActive || (removals != nil && removals[i])
		e, ok := w.newEntity(d.Name, d.ChangeNumber, removed, *d)
		if !ok {
			skipped = append(skipped, d.Name)
			continue
		}
		ents = append(ents, e)
	}
	return ents, skipped
}

// newEntity serializes a DTO. The removed flag comes from the DTO status, never from the list it arrived in.
func (w *Writer) newEntity(name string, cn int64, removed bool, dto interface{}) (entity, bool) {
	raw, err := w.marshal(dto)
	if err != nil {
		w.reject(fmt.Sprintf("redis state: cannot serialize %q, skipped: %s", name, err.Error()))
		return entity{}, false
	}
	return entity{name: name, cn: cn, removed: removed, json: string(raw)}, true
}

// buildEntityArgs returns the ARGV of FlagScript and RuleBasedSegmentScript: fingerprint, retentionMs,
// cursor field, cursor, count, then name, cn, removed (0|1), json for every entity. The reader's tests run these
// args against the scripts, so the positions here and in script.go must change together.
func buildEntityArgs(fingerprint string, retentionMs int64, cursorField string, cursor int64, ents []entity) []interface{} {
	args := make([]interface{}, 0, 5+4*len(ents))
	args = append(args, fingerprint, retentionMs, cursorField, cursor, len(ents))
	for _, e := range ents {
		removed := "0"
		if e.removed {
			removed = "1"
		}
		args = append(args, e.name, e.cn, removed, e.json)
	}
	return args
}

// buildSegmentArgs returns the ARGV of SegmentScript: fingerprint, retentionMs, name, cursor field, cursor,
// nAdd, nRemove, then the (key, cn) pairs of the additions and then of the removals.
func buildSegmentArgs(fingerprint string, retentionMs int64, name, cursorField string, cursor int64, adds, rems []segmentKey) []interface{} {
	args := make([]interface{}, 0, 7+2*(len(adds)+len(rems)))
	args = append(args, fingerprint, retentionMs, name, cursorField, cursor, len(adds), len(rems))
	for _, k := range adds {
		args = append(args, k.key, k.cn)
	}
	for _, k := range rems {
		args = append(args, k.key, k.cn)
	}
	return args
}

// DecodeFlag rebuilds a feature flag from the stored entity. data is the JSON from the data hash, cn is the index
// score and removed is membership of the removed set. The index score is authoritative over any change number
// inside the JSON. A removed entity is returned with status ARCHIVED.
func DecodeFlag(data []byte, cn int64, removed bool) (dtos.SplitDTO, error) {
	var d dtos.SplitDTO
	if err := json.Unmarshal(data, &d); err != nil {
		return dtos.SplitDTO{}, fmt.Errorf("decoding flag entity: %w", err)
	}
	d.ChangeNumber = cn
	if removed {
		d.Status = statusArchived
	}
	return d, nil
}

// DecodeRuleBasedSegment is DecodeFlag for rule-based segments
func DecodeRuleBasedSegment(data []byte, cn int64, removed bool) (dtos.RuleBasedSegmentDTO, error) {
	var d dtos.RuleBasedSegmentDTO
	if err := json.Unmarshal(data, &d); err != nil {
		return dtos.RuleBasedSegmentDTO{}, fmt.Errorf("decoding rule-based segment entity: %w", err)
	}
	d.ChangeNumber = cn
	if removed {
		d.Status = statusArchived
	}
	return d, nil
}

// ParseCursor parses a cursor value read from the meta hash
func ParseCursor(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing cursor %q: %w", s, err)
	}
	return v, nil
}

func chunkCount(n, size int) int {
	if n == 0 {
		return 1
	}
	return (n + size - 1) / size
}

// span returns the part of list (which starts at position offset of the overall sequence) inside [lo, hi)
func span[T any](list []T, lo, hi, offset int) []T {
	s := min(max(lo-offset, 0), len(list))
	e := min(max(hi-offset, 0), len(list))
	return list[s:e]
}

// groupFreeze is the in-memory state of one flag or rule-based segment group (D19). names holds the entities refused
// or skipped and not yet written cleanly. last is the last cursor sent with a real value. While names is not empty
// the group sends last, so the stored cursor stays behind the skipped entity. whole is set when the skipped set grows past MaxPendingChanges: the names are dropped and the group stays frozen until a resync releases it. Lost on restart (mirror-only gap).
type groupFreeze struct {
	names   map[string]struct{}
	last    int64
	hasLast bool
	whole   bool
}

// releaseGroup drops the skipped names and the whole-group freeze of the group that owns cursorField (D19 rule b). A resync
// calls it before it rewrites the group, so a name skipped again during that resync freezes the group once more. It returns
// the previous state so restoreGroup can put it back if the write fails.
func (w *Writer) releaseGroup(cursorField string) groupFreeze {
	g := w.freezeOf(cursorField)
	old := *g
	g.names = map[string]struct{}{}
	g.whole = false
	return old
}

// restoreGroup merges the state returned by releaseGroup back into the group after a failed write
func (w *Writer) restoreGroup(cursorField string, old groupFreeze) {
	g := w.freezeOf(cursorField)
	if old.whole {
		g.names = map[string]struct{}{}
		g.whole = true
		return
	}
	for name := range old.names {
		g.names[name] = struct{}{}
	}
}

// freezeOf returns the state of the group that owns cursorField, creating it on first use
func (w *Writer) freezeOf(cursorField string) *groupFreeze {
	if w.groups == nil {
		w.groups = map[string]*groupFreeze{}
	}
	g := w.groups[cursorField]
	if g == nil {
		g = &groupFreeze{names: map[string]struct{}{}}
		w.groups[cursorField] = g
	}
	return g
}

// pendingOutside reports whether a skipped entity of the group is not in ents, that is, is not rewritten by this write
func (g *groupFreeze) pendingOutside(ents []entity) bool {
	in := make(map[string]struct{}, len(ents))
	for _, e := range ents {
		in[e.name] = struct{}{}
	}
	for name := range g.names {
		if _, ok := in[name]; !ok {
			return true
		}
	}
	return false
}

// cursorToSend returns the cursor for the last chunk of a write. A frozen write sends the last written cursor, or
// noCursor when the process has not written one yet.
func (g *groupFreeze) cursorToSend(cursor int64, frozen bool) int64 {
	if !frozen {
		return cursor
	}
	if g.hasLast {
		return g.last
	}
	return noCursor
}

// addSkip records a skipped entity name. Past limit names the set is discarded and the group freezes as a whole (D19 cap).
func (g *groupFreeze) addSkip(name string, limit int) {
	if g.whole {
		return
	}
	g.names[name] = struct{}{}
	if len(g.names) > limit {
		g.names = map[string]struct{}{}
		g.whole = true
	}
}

// dropped freezes the whole group after a change was dropped (D36): the stored cursor must not pass the lost change, and a resync releases the freeze
func (g *groupFreeze) dropped() {
	g.names = map[string]struct{}{}
	g.whole = true
}

// written records a successful send: its entities are clean, and a real cursor becomes the last written one
func (g *groupFreeze) written(part []entity, cur int64) {
	for _, e := range part {
		delete(g.names, e.name)
	}
	if cur != noCursor {
		g.last, g.hasLast = cur, true
	}
}

// writeEntities writes flags or rule-based segments in chunks. Only the last call carries the real cursor, and
// not even that one while the group is frozen, so the stored cursor is never ahead of the stored data.
// A chunk refused with PROXY_BAD_ARGS is retried one entity at a time, so only the refused entity is skipped.
func (w *Writer) writeEntities(what, script string, keys []string, cursorField string, ents []entity, cursor int64, buildSkipped []string) (bool, error) {
	g := w.freezeOf(cursorField)
	skipped := len(buildSkipped) > 0
	for _, name := range buildSkipped {
		g.addSkip(name, w.opts.MaxPendingChanges)
	}
	pending := g.pendingOutside(ents)
	n := chunkCount(len(ents), w.opts.ChunkSize)
	refused := false
	send := func(part []entity, last bool) error {
		cur := noCursor
		if last {
			cur = g.cursorToSend(cursor, skipped || pending || g.whole || w.overflowFrozen())
		}
		args := buildEntityArgs(w.fingerprint, w.opts.Retention.Milliseconds(), cursorField, cur, part)
		if err := w.eval(what, script, keys, args); err != nil {
			if errors.Is(err, errDropped) {
				g.dropped()
			}
			return err
		}
		g.written(part, cur)
		return nil
	}
	skipEntity := func(name string) {
		skipped = true
		g.addSkip(name, w.opts.MaxPendingChanges)
		w.reject(fmt.Sprintf("redis state: %s entity %q was rejected by the script because of inconsistent arguments, skipped", what, name))
	}
	for i := 0; i < n; i++ {
		part := span(ents, i*w.opts.ChunkSize, (i+1)*w.opts.ChunkSize, 0)
		last := i == n-1
		err := send(part, last)
		if err == nil {
			continue
		}
		if !errors.Is(err, errChunkRejected) {
			return refused, err
		}
		refused = true
		if len(part) < 2 {
			skipped = true
			for _, e := range part {
				skipEntity(e.name)
			}
			continue
		}
		for j := range part {
			err := send(part[j:j+1], last && j == len(part)-1)
			if errors.Is(err, errChunkRejected) {
				skipEntity(part[j].name)
				continue
			}
			if err != nil {
				return refused, err
			}
		}
	}
	if skipped {
		w.skipsSeen = true
		w.logger.Error(fmt.Sprintf("redis state: %s skipped some data, the stored cursor was not advanced; the skipped data is not retried until the next resync", what))
	}
	return refused, nil
}

// writeSegment writes the keys of one segment in chunks; the same cursor and isolation rules as writeEntities apply,
// and at least one chunk is always sent so the segment name and the cursor are written even for an empty change.
func (w *Writer) writeSegment(name string, adds, rems []segmentKey, cursor int64, skipped bool) (bool, error) {
	total := len(adds) + len(rems)
	refused := false
	size := w.opts.ChunkSize
	n := chunkCount(total, size)
	keys := w.keys.SegmentKeys(name)
	field := w.keys.SegmentCursorField(name)
	g := w.freezeOf(field)
	send := func(a, r []segmentKey, last bool) error {
		cur := noCursor
		if last {
			cur = g.cursorToSend(cursor, skipped || g.whole || w.overflowFrozen())
		}
		args := buildSegmentArgs(w.fingerprint, w.opts.Retention.Milliseconds(), name, field, cur, a, r)
		if err := w.eval("segment write", SegmentScript, keys, args); err != nil {
			if errors.Is(err, errDropped) {
				g.dropped()
			}
			return err
		}
		g.written(nil, cur)
		return nil
	}
	skipKey := func(k segmentKey) {
		skipped = true
		w.skipsSeen = true
		w.reject(fmt.Sprintf("redis state: segment %s has a key rejected by the script because of inconsistent arguments, skipped", name))
	}
	for i := 0; i < n; i++ {
		lo, hi := i*size, (i+1)*size
		a := span(adds, lo, hi, 0)
		r := span(rems, lo, hi, len(adds))
		last := i == n-1
		err := send(a, r, last)
		if err == nil {
			continue
		}
		if !errors.Is(err, errChunkRejected) {
			return refused, err
		}
		refused = true
		if len(a)+len(r) < 2 {
			skipped = true
			for _, k := range a {
				skipKey(k)
			}
			for _, k := range r {
				skipKey(k)
			}
			continue
		}
		for j := range a {
			err := send(a[j:j+1], nil, last && len(r) == 0 && j == len(a)-1)
			if errors.Is(err, errChunkRejected) {
				skipKey(a[j])
				continue
			}
			if err != nil {
				return refused, err
			}
		}
		for j := range r {
			err := send(nil, r[j:j+1], last && j == len(r)-1)
			if errors.Is(err, errChunkRejected) {
				skipKey(r[j])
				continue
			}
			if err != nil {
				return refused, err
			}
		}
	}
	if skipped {
		w.logger.Error(fmt.Sprintf("redis state: segment %s skipped some data, the stored cursor was not advanced; the skipped data is not retried until the next resync", name))
	}
	return refused, nil
}
