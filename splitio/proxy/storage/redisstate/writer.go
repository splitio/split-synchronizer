package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// Upper bounds: a Lua script of unbounded size blocks Redis past lua-time-limit.
	// MaxChunkSize caps the number of entities sent in one Lua script call.
	MaxChunkSize = 10000
	// MaxPurgeBatch caps the number of keys removed per purge batch.
	MaxPurgeBatch = 5000
	// MaxStopTimeoutMs caps how long Stop waits for the worker to drain, in milliseconds.
	MaxStopTimeoutMs = 60000
	// MaxToken is the largest lease fencing token. Lua tonumber and isint are exact only up to 15 digits, so a larger
	// token would be refused as a bad argument instead of failing at construction.
	MaxToken = 999999999999999

	// noCursor is the cursor argument that makes a script leave the stored cursor untouched
	noCursor = int64(-1)
)

var (
	// ErrEnvironmentMismatch is returned when Redis holds state of another environment/configuration
	ErrEnvironmentMismatch = errors.New("redis state belongs to a different environment or configuration")
	// ErrFenced is returned when Redis rejects a write because this writer no longer holds the lease (stale token,
	// another owner, or the lease key is gone). The writer stops for good; a new writer with a new lease is needed
	ErrFenced = errors.New("redis state writer lease was lost, the writer is fenced")
	// ErrInvalidOptions is returned when the writer is built with an invalid option or dependency
	ErrInvalidOptions = errors.New("invalid redis state writer options")
	// ErrInvalidSegmentName is returned by DeleteSegmentKeys for an empty name or one with glob characters
	ErrInvalidSegmentName = errors.New("redis state segment name is empty or contains glob characters")

	// ErrWorkerRunning is returned by DeleteSegmentKeys while the worker runs: a repair that interleaves with a live
	// segment write could leave a current cursor over partial keys
	ErrWorkerRunning = errors.New("redis state writer is running")

	errStopped       = errors.New("redis state writer stopped")
	errChunkRejected = errors.New("redis state chunk rejected by the script")
	// errDropped is returned once transient retries ran out: the change is not in Redis, which is behind
	errDropped = errors.New("redis state change dropped after transient errors, Redis is behind")
)

const (
	// maxConsecutiveRefusals bounds the single-entity calls after a refused chunk: when this many in a row are refused too,
	// the cause is not one bad entity, so the rest of the chunk is skipped and the group is frozen
	maxConsecutiveRefusals = 5
	// maxResyncDrops bounds the automatic resync retries after consecutive resyncs dropped on transient errors
	maxResyncDrops = 10
)

// errClass is the class of a Redis error
type errClass int

const (
	errTransient errClass = iota
	errEnvMismatch
	errFenced
	errPermanent
	errUnknown
)

// String returns the class name used in log and error text
func (c errClass) String() string {
	switch c {
	case errTransient:
		return "transient"
	case errEnvMismatch:
		return "environment mismatch"
	case errFenced:
		return "fenced"
	case errPermanent:
		return "permanent"
	default:
		return "unknown"
	}
}

// Marker lists. A marker counts only as the first word of the error text, after an optional "ERR " prefix and the
// Lua wrapper (see errHead), so a marker echoed inside an entity name does not change the class.
var (
	transientHeads = []string{"READONLY", "LOADING", "OOM", "TRYAGAIN", "CLUSTERDOWN", "MASTERDOWN", "MISCONF", "BUSY", "MOVED", "ASK", "NOREPLICAS", "EXECABORT", "NOAUTH", "PROXY_META_WRONGTYPE"}
	// transientTexts are matched by their exact full text (the go-redis texts include the "redis: " prefix)
	transientTexts = []string{"ERR max number of clients reached", "redis: connection pool timeout", "redis: client is closed"}
	permanentHeads = []string{"PROXY_BAD_ARGS", "PROXY_DATA_WRONGTYPE", "WRONGTYPE", "CROSSSLOT", "NOPERM"}
)

// classify maps a Redis or client error to its class. The order is the table: transient first (even inside a Lua
// wrapper), then the environment mismatch and fenced markers, then permanent (a permanent marker or a Lua runtime error), and
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
	case ErrorHasMarker(err, FencedMarker):
		return errFenced
	case strings.Contains(msg, "@user_script:"), slices.Contains(permanentHeads, head), evalUnavailable(msg):
		return errPermanent
	}
	return errUnknown
}

// evalUnavailable reports a reply that says EVAL cannot run on this server or for this user: retrying cannot help.
// It matches the start of the reply only, so entity text echoed inside an error does not count.
func evalUnavailable(msg string) bool {
	body := strings.ToLower(strings.TrimPrefix(msg, "ERR "))
	return strings.HasPrefix(body, "unknown command") || (strings.HasPrefix(body, "eval") && strings.Contains(body, "disabled"))
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
// Healthy is false from the first lost or refused change until a resync with a Source completes cleanly, even when later writes succeed.
// DroppedChanges counts live changes lost before reaching Redis (overflow, retries exhausted, Stop, mismatch, worker panic), not resync source read failures.
// Resyncs counts overflow events (every time queued changes were dropped) and resyncs scheduled after a refused live chunk; the startup resync is not counted.
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
	MaxPendingChanges int           // default 3000000 (entities + keys queued; approximate, the in-flight change and merge index are not counted)
	ChunkSize         int           // default 1000 (entities or keys per script call)
	PurgeBatch        int           // default 500 (entries removed per PurgeScript call), 1..5000
	StopTimeoutMs     int64         // default 10000 (drain bound on Stop only; a blocked Redis call is bounded by the client timeouts), 1..60000
	Owner             string        // lease owner sent to the fenced scripts; required, no default
	Token             int64         // lease fencing token sent to the fenced scripts; required, must be > 0, no default
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
	removals   []bool // true for entities from the removal list of a PutFlags or PutRuleBasedSegments call
	segment    string
	adds       []segmentKey
	rems       []segmentKey
	// namePos (flags, rule-based segments) and keyPos (segments) map an entity name or key to its position. Built
	// lazily on the first merge, then updated on every merge, so a merge costs O(size of the merged change).
	namePos map[string]int
	pos     map[string]keyPos
}

// keyPos locates a segment key in change.adds (add true) or change.rems.
type keyPos struct {
	add bool
	idx int
}

// groupKey identifies the queue group of a change: flags, rule-based segments, or one segment name.
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
// The worker holds mtx only to read or change queue and
// status state, never across Source calls, Eval, HGetAll or a wait.
type Writer struct {
	// overflow and overflowEpoch are guarded by mtx. overflow freezes the cursors of every group until a resync
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
	groups      map[string]*groupFreeze // in-memory group state, keyed by cursor field; lazily created by freezeOf

	now     func() time.Time
	wait    func(time.Duration) bool
	marshal func(v interface{}) ([]byte, error)

	signal   chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	// abort is set by Abort: work ends at once, without the drain window. Read without mtx, so it is atomic
	abort atomic.Bool
	// drainUntil is the drain deadline, written by Stop before w.stop closes and read only after the close is observed
	drainUntil time.Time

	mtx        sync.Mutex
	started    bool
	queue      []*change
	queuedSize int
	// groupIdx maps a group to its last queued change. A change leaves the index when it is popped or the queue is cleared.
	groupIdx    map[groupKey]*change
	inflight    *change
	needResync  bool
	canResync   bool
	resyncSince time.Time
	mismatch    bool
	// fenced is set once Redis rejects a write for a lost lease: the worker stops, nothing is queued again
	fenced bool
	// dead is set by recoverWorker after a worker panic: nothing is queued again, the mirror stays stopped
	dead          bool
	refusedLogged bool
	// deadRefusedLogged limits the Error line for changes refused after a worker panic to the first one
	deadRefusedLogged bool
	// degraded is sticky: a change was lost or refused and Redis is behind the source. Only a clean resync clears it
	// (clearOverflowIfUnchanged) or Reset; a later successful write does not, so Status stays unhealthy while data is missing.
	degraded       bool
	degradedReason string
	// stopRefusedLogged is guarded by mtx; woken is owned by the worker goroutine (see sleep)
	stopRefusedLogged bool
	woken             bool
	stats             Status
}

// NewWriter validates opts (all > 0, else an error wrapping ErrInvalidOptions) and builds a Writer. It does not touch Redis.
// The writer assumes a single-master Redis (standalone or Sentinel-managed master, both accepted by common/redis.go):
// the client does not prefix the Eval KEYS in cluster mode, so cluster deployments are unsupported for now.
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
	case opts.Owner == "" || opts.Token <= 0 || opts.Token > MaxToken:
		return nil, fmt.Errorf("%w: owner is required and token must be 1..%d", ErrInvalidOptions, MaxToken)
	case keys.prefix != client.Prefix():
		return nil, fmt.Errorf("%w: keys prefix %q does not match the client prefix %q", ErrInvalidOptions, keys.prefix, client.Prefix())
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

// Start(nil) is for tests: with no Source no resync can run, so a drop freeze is never lifted. Production always passes a Source.
// Start runs the startup probe, then launches the single background worker. The worker first calls source.Resync (if source != nil), then drains the queue.
// A probe that fails with a script error returns a minimum-version InitError and starts no worker.
// A namespace has one writer at a time: the supervisor holds the writer lease and fences the Writer with its token, and
// replicas that share an API key share the namespace by design.
func (w *Writer) Start(source Source) error {
	if !hasSource(source) {
		source = nil
	}
	if w.stopped() {
		return nil
	}
	w.mtx.Lock()
	alreadyStarted := w.started
	w.mtx.Unlock()
	if alreadyStarted {
		w.logger.Warning("redis state writer already started")
		return nil
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

// sharedNamespaceWindow is how recent meta.updatedAtMs must be for Start to log that the namespace was just written
const sharedNamespaceWindow = time.Minute

// warnIfNamespaceInUse logs at Debug when the namespace was written to within sharedNamespaceWindow. A shared
// namespace is the supported topology: the lease keeps one writer at a time, so this is expected after every takeover,
// new term or restart and is not a warning. It reads the meta hash once on the worker goroutine and never fails Start.
func (w *Writer) warnIfNamespaceInUse() {
	fields, err := w.client.HGetAll(w.keys.MetaUnprefixed())
	if err != nil {
		w.logger.Debug(fmt.Sprintf("redis state: could not read the meta hash to check the namespace: %v", err))
		return
	}
	raw, ok := fields[UpdatedAtField]
	if !ok || raw == "" {
		return
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		w.logger.Debug("redis state: unreadable meta updatedAtMs, skipping the shared namespace check")
		return
	}
	if ms <= 0 {
		return
	}
	if age := w.now().Sub(time.UnixMilli(ms)); age >= 0 && age < sharedNamespaceWindow {
		w.logger.Debug(fmt.Sprintf("redis state: the namespace was written %s ago, by the previous writer term or another proxy of this namespace", age.Round(time.Second)))
	}
}

// probe runs startProbeScript once under the namespace. A non-transient Redis error reply fails Start with an
// InitError: a script-support reply (unknown command, or a write after TIME refused on Redis before 5.0) gets the
// minimum-version message, any other reply (auth, permission, ...) keeps its own text. Transport and timeout errors
// are not reply errors: they are logged and the worker retries through its normal path.
func (w *Writer) probe() error {
	err := w.client.Eval(startProbeScript, []string{w.keys.base + ".probe"})
	if err == nil {
		return nil
	}
	var reply interface{ RedisError() }
	if errors.As(err, &reply) && classify(err) != errTransient {
		if isScriptUnsupportedReply(err) {
			return common.NewInitError(fmt.Errorf("redis state writer requires Redis 5.0 or later (scripts call TIME then write): %w", err), common.ExitRedisInitializationFailed)
		}
		return common.NewInitError(fmt.Errorf("redis state writer start probe rejected by Redis: %w", err), common.ExitRedisInitializationFailed)
	}
	w.logger.Warning("redis state writer start probe failed, the worker will retry: ", err)
	return nil
}

// isScriptUnsupportedReply reports whether a Redis error reply means the server cannot run the probe script:
// EVAL missing, or a write after a non-deterministic command refused (Redis before 5.0 without script effects replication)
func isScriptUnsupportedReply(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") || strings.Contains(msg, "write commands not allowed after non deterministic commands")
}

// Stop drains the queue for at most StopTimeoutMs, then drops what is left and returns after the worker exits. New
// input is refused from the call on. Dropped changes never had their cursors written, so Redis stays behind the source.
func (w *Writer) Stop() {
	w.stopOnce.Do(func() {
		w.drainUntil = w.now().Add(time.Duration(w.opts.StopTimeoutMs) * time.Millisecond)
		close(w.stop)
	})
	dropped := w.waitAndDrop()
	msg := fmt.Sprintf("redis state writer stopped, %d queued changes dropped (drain bound %d ms), their cursors were not written so Redis stays behind the source", dropped, w.opts.StopTimeoutMs)
	if dropped > 0 {
		w.logger.Error(msg)
	} else {
		w.logger.Info(msg)
	}
}

// Dead reports that this Writer can no longer write: its worker died (recoverWorker), it is in the environment
// mismatch state, or Redis fenced it. The supervisor checks it at every renew tick and steps down when it is true.
func (w *Writer) Dead() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.dead || w.mismatch || w.fenced
}

// Abort stops the writer without a drain: pending and in-flight changes are dropped and the worker is stopped (a Stop
// already waiting on the drain returns early). It returns after the worker has exited, so it waits for the in-flight
// Eval or Source callback to return; that wait is bounded by the client timeouts. It is idempotent and safe to call
// together with Stop. It logs at Error only when changes were dropped.
func (w *Writer) Abort() {
	w.abort.Store(true)
	w.stopOnce.Do(func() {
		w.drainUntil = w.now()
		close(w.stop)
	})
	dropped := w.waitAndDrop()
	msg := fmt.Sprintf("redis state writer aborted, %d queued changes dropped without a drain, their cursors were not written so Redis stays behind the source", dropped)
	if dropped > 0 {
		w.logger.Error(msg)
	} else {
		w.logger.Info(msg)
	}
}

// waitAndDrop waits for the worker to exit when it was started, then drops the queue and the in-flight change and
// counts them. It returns the number dropped.
func (w *Writer) waitAndDrop() int {
	w.mtx.Lock()
	started := w.started
	w.mtx.Unlock()
	if started {
		<-w.done
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()
	dropped := len(w.queue)
	if w.inflight != nil {
		dropped++
		w.inflight = nil
	}
	w.clearQueueLocked()
	w.stats.DroppedChanges += int64(dropped)
	return dropped
}

// RequestResync asks the worker to run a full resync of Redis against the Source given to Start, through the same
// path as the startup, refused-chunk and overflow resyncs. Call it once the Source is populated (for example after
// the first successful sync) so state skipped on a cold boot can be repaired. It never blocks and is safe to
// call from any goroutine any number of times: calls made while a resync is scheduled but not started coalesce into
// one. It does nothing before Start, when Start got no Source, after Stop, or after an environment mismatch. A
// requested resync is not counted in Status.Resyncs.
func (w *Writer) RequestResync() {
	w.mtx.Lock()
	ok := w.canResync && !w.mismatch && !w.dead && !w.fenced && !w.stopped()
	if ok {
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	if ok {
		w.wake()
	}
}

// resetBatch caps the data keys one ResetScript 'del' call deletes, and the meta fields one 'meta' call removes. It keeps
// each call under the Lua unpack limit of the script.
const resetBatch = 500

// Reset deletes the state of this environment namespace it knows of and clears the mismatch flag. It requires the lease
// (Owner and Token): every step is a fenced ResetScript call. Meta keeps the fence and gets the writer's own fingerprint
// back at the end, so a foreign fingerprint is taken over. The reader is invalid from the first step on, because the
// flag and rule-based segment cursors are removed before any data key. Meta itself is never deleted. When the lease
// moves it stops at once and returns an error wrapping ErrFenced, deleting nothing after the refused call. Call before
// Start, only when Initialization.ForceFreshStartup is set.
func (w *Writer) Reset() error {
	if err := w.resetFenced(); err != nil {
		return fmt.Errorf("deleting the redis state of environment %s: %w", w.keys.Env(), err)
	}
	w.mtx.Lock()
	w.clearQueueLocked()
	w.mismatch = false
	w.refusedLogged = false
	w.degraded = false
	w.degradedReason = ""
	w.stats.Healthy = true
	w.stats.LastError = ""
	w.stats.FailedRounds = 0
	w.mtx.Unlock()
	return nil
}

// resetFenced runs the Reset steps, every call fenced through ResetScript. The meta hash is read unprefixed, as before.
// The data keys are named through w.keys, because Eval keys get no prefix. The meta key is never in a data list.
//  1. HGetAll meta, read only.
//  2. 'meta' calls HDEL every meta field except fence, fingerprint and the segment cursors (flags.till and rbs.till
//     included), so the reader is invalid before any data key goes.
//  3. 'del' calls, each with at most resetBatch data keys: the 7 fixed keys first, then each segment's three keys
//     together with its cursor field as ARGV, so a segment and its cursor leave in the same call and never straddle two.
//     An aborted Reset leaves only the segments not yet deleted, each still with its cursor, so a rerun deletes them.
//  4. 'fingerprint' sets meta fingerprint to the writer's own, taking over a foreign one.
func (w *Writer) resetFenced() error {
	meta, err := w.client.HGetAll(w.keys.Unprefixed().Meta())
	if err != nil {
		return err
	}
	owner, token := w.opts.Owner, w.opts.Token
	lease := []string{w.keys.Lease(), w.keys.Meta()}

	var metaFields []string
	cursors := make(map[string]string)
	for field := range meta {
		if field == "fence" || field == "fingerprint" {
			continue
		}
		if name, ok := segmentNameOfCursorField(field); ok {
			cursors[name] = field
			continue
		}
		metaFields = append(metaFields, field)
	}
	for start := 0; start < len(metaFields); start += resetBatch {
		end := min(start+resetBatch, len(metaFields))
		args := []interface{}{owner, token, "meta"}
		for _, field := range metaFields[start:end] {
			args = append(args, field)
		}
		if err := w.resetEval(lease, args...); err != nil {
			return err
		}
	}

	// a unit is the keys that must leave together with the fields it carries; a segment is never split
	type unit struct {
		keys   []string
		fields []string
	}
	k := w.keys
	var units []unit
	for _, key := range []string{
		k.PurgeLast(), k.FlagData(), k.FlagIndex(), k.FlagRemoved(),
		k.RuleBasedSegmentData(), k.RuleBasedSegmentIndex(), k.RuleBasedSegmentRemoved(),
	} {
		units = append(units, unit{keys: []string{key}})
	}
	for _, name := range slices.Sorted(maps.Keys(cursors)) {
		units = append(units, unit{
			keys:   []string{k.SegmentActive(name), k.SegmentTombstones(name), k.SegmentRemovalClock(name)},
			fields: []string{cursors[name]},
		})
	}

	var keys, fields []string
	flush := func() error {
		args := []interface{}{owner, token, "del"}
		for _, field := range fields {
			args = append(args, field)
		}
		err := w.resetEval(append(append([]string{}, lease...), keys...), args...)
		keys, fields = keys[:0], fields[:0]
		return err
	}
	for _, u := range units {
		if len(keys)+len(u.keys) > resetBatch {
			if err := flush(); err != nil {
				return err
			}
		}
		keys = append(keys, u.keys...)
		fields = append(fields, u.fields...)
	}
	if len(keys) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}

	return w.resetEval(lease, owner, token, "fingerprint", w.fingerprint)
}

// resetEval runs one ResetScript call. A fenced refusal is returned wrapping ErrFenced and is never retried.
func (w *Writer) resetEval(keys []string, args ...interface{}) error {
	err := w.client.Eval(ResetScript, keys, args...)
	if ErrorHasMarker(err, FencedMarker) {
		return fmt.Errorf("%w: %w", ErrFenced, err)
	}
	return err
}

// DeleteSegmentKeys deletes the active, tombstone and removal clock keys of one segment and resets its cursor to -1
// in the same script, so a restart never reads a current cursor over a partial segment. No other key is touched.
// The segment write that follows rewrites the segment. It uses the worker's retry state, so it runs before Start or after
// Stop only; while the worker runs it returns ErrWorkerRunning and deletes nothing. The resync repairs through deleteSegmentKeys.
func (w *Writer) DeleteSegmentKeys(name string) error {
	if w.workerRunning() {
		return ErrWorkerRunning
	}
	return w.deleteSegmentKeys(name)
}

// workerRunning reports whether Start launched the worker and it has not exited yet
func (w *Writer) workerRunning() bool {
	w.mtx.Lock()
	started := w.started
	w.mtx.Unlock()
	if !started {
		return false
	}
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// deleteSegmentKeys is DeleteSegmentKeys without the running check, for the worker's own repair
func (w *Writer) deleteSegmentKeys(name string) error {
	if !validSegmentName(name) {
		return ErrInvalidSegmentName
	}
	field := w.keys.SegmentCursorField(name)
	err := w.retry("segment key delete", func() error {
		return w.client.Eval(SegmentRepairScript, append(w.keys.SegmentKeys(name), w.keys.Lease()), w.fingerprint, field, w.opts.Owner, w.opts.Token)
	})
	if err == nil {
		// the stored cursor is -1 now, so the last cursor this process sent no longer describes Redis
		w.freezeOf(field).hasLast = false
	}
	return err
}

// segmentNameOfCursorField returns the segment name of a meta field "seg.<name>.till"
func segmentNameOfCursorField(field string) (string, bool) {
	const prefix, suffix = "seg.", ".till"
	if len(field) <= len(prefix)+len(suffix) || !strings.HasPrefix(field, prefix) || !strings.HasSuffix(field, suffix) {
		return "", false
	}
	return field[len(prefix) : len(field)-len(suffix)], true
}

// The Put methods copy the slices but not the DTOs they point to: callers must treat DTOs as immutable after Put, because marshalling happens on the worker.
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
	rems := make([]segmentKey, 0, len(toRemove))
	// a key in both lists is kept as an add; the lookup is only needed when both lists have entries
	var added map[string]struct{}
	if len(toAdd) > 0 && len(toRemove) > 0 {
		added = make(map[string]struct{}, len(toAdd))
		for _, k := range toAdd {
			added[k] = struct{}{}
		}
	}
	for _, k := range toRemove {
		if _, both := added[k]; both {
			continue
		}
		rems = append(rems, segmentKey{key: k, cn: cn})
	}
	w.enqueue(&change{kind: kindSegment, cn: cn, segment: name, adds: adds, rems: rems, size: len(adds) + len(rems)})
}

// Status returns a point-in-time view of the writer.
func (w *Writer) Status() Status {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	s := w.stats
	if w.degraded {
		s.Healthy = false
		if s.LastError == "" {
			s.LastError = w.degradedReason
		}
	}
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
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return !v.IsNil()
	}
	return true
}

func validSegmentName(name string) bool {
	return name != "" && !strings.ContainsAny(name, globChars)
}

// refuse counts and reports a change refused because of an environment mismatch or a dead worker
func (w *Writer) refuse() bool {
	w.mtx.Lock()
	msg, level := w.refuseLocked()
	w.mtx.Unlock()
	w.logRefusal(msg, level)
	return msg != ""
}

const (
	refusalNone = iota
	refusalFirst
	refusalRepeat
)

// Accepted trade-off: both states last until an operator acts (new namespace, forced fresh start, restart), so only the
// first refusal logs at Error. Repeats log at Debug and are counted in RefusedChanges; Status shows Healthy=false and LastError.
// refuseLocked counts a change refused by a mismatch or a dead worker and returns the log line and whether it is the
// first one of its kind (Error) or a repeat (Debug). It returns "" when the change is accepted. The caller holds mtx
// and logs after the unlock.
func (w *Writer) refuseLocked() (string, int) {
	switch {
	case w.mismatch:
		w.stats.RefusedChanges++
		if !w.refusedLogged {
			w.refusedLogged = true
			return "redis state: change refused because Redis holds the state of another environment or configuration; further refusals are logged at debug level", refusalFirst
		}
		return "redis state: change refused (environment mismatch)", refusalRepeat
	case w.fenced:
		w.stats.RefusedChanges++
		return "redis state: change refused because the writer lease was lost (fenced)", refusalRepeat
	case w.dead:
		w.stats.RefusedChanges++
		if !w.deadRefusedLogged {
			w.deadRefusedLogged = true
			return "redis state: change refused because the writer worker stopped after a panic, Redis is no longer updated; further refusals are logged at debug level", refusalFirst
		}
		return "redis state: change refused (writer worker stopped)", refusalRepeat
	}
	return "", refusalNone
}

func (w *Writer) logRefusal(msg string, level int) {
	switch level {
	case refusalFirst:
		w.logger.Error(msg)
	case refusalRepeat:
		w.logger.Debug(msg)
	}
}

func (w *Writer) reject(msg string) {
	w.rejectN(1, msg)
}

// rejectN counts n rejected entities and logs msg once; an empty msg logs nothing
func (w *Writer) rejectN(n int, msg string) {
	w.mtx.Lock()
	w.stats.RejectedChanges += int64(n)
	w.mtx.Unlock()
	if msg != "" {
		w.logger.Error(msg)
	}
}

func (w *Writer) enqueue(c *change) {
	c.enqueuedAt = w.now()
	var overflowMsg string

	w.mtx.Lock()
	if w.stopped() {
		// once Stop is called the change is refused, so a busy source cannot extend the drain
		w.stats.DroppedChanges++
		first := !w.stopRefusedLogged
		w.stopRefusedLogged = true
		w.mtx.Unlock()
		if first {
			w.logger.Error("redis state: change refused after Stop, the writer is draining or stopped; further refusals are not logged")
		}
		return
	}
	if msg, level := w.refuseLocked(); msg != "" {
		w.mtx.Unlock()
		w.logRefusal(msg, level)
		return
	}
	if len(w.queue) > 0 && w.sizeAfterLocked(c) > w.opts.MaxPendingChanges {
		dropped := len(w.queue)
		w.clearQueueLocked()
		w.stats.DroppedChanges += int64(dropped)
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
		w.markDegradedLocked(fmt.Sprintf("more than %d changes pending, %d queued changes dropped", w.opts.MaxPendingChanges, dropped))
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

// logOverflow logs an overflow decided under mtx. It runs after the unlock, so the logger is never called under the lock.
func (w *Writer) logOverflow(msg string) {
	if msg != "" {
		w.logger.Error(msg)
	}
}

// mergeLocked folds c into the last queued change q of the same group. q is never the in-flight change:
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
// appended as a new change. The queue contents are not changed. The caller holds mtx.
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
		if _, ok := q.pos[k.key]; !ok && !seenName(seen, k.key) {
			fresh++
		}
	}
	for _, k := range c.adds {
		if _, ok := q.pos[k.key]; !ok && !seenName(seen, k.key) {
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

// indexNames builds the name-to-position map of a flags or rule-based segments change once.
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

// indexKeys builds the key-to-position map of a segment change once.
func (q *change) indexKeys() {
	if q.pos != nil {
		return
	}
	q.pos = make(map[string]keyPos, len(q.adds)+len(q.rems))
	for i := range q.rems {
		q.pos[q.rems[i].key] = keyPos{add: false, idx: i}
	}
	for i := range q.adds {
		q.pos[q.adds[i].key] = keyPos{add: true, idx: i}
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
				q.removals[p] = removalAt(c.removals, i)
			}
			continue
		}
		q.namePos[d.Name] = len(q.flags)
		q.flags = append(q.flags, d)
		q.removals = append(q.removals, removalAt(c.removals, i))
	}
	q.size = len(q.flags)
}

// removalAt reports whether entity i is a removal; an index past the end of rem counts as false
func removalAt(rem []bool, i int) bool {
	return i < len(rem) && rem[i]
}

// mergeRuleBased keeps, per rule-based segment name, the entry with the higher change number; at an equal cn the later arrival wins.
func (q *change) mergeRuleBased(c *change) {
	q.indexNames()
	for i := range c.rbs {
		d := c.rbs[i]
		if p, ok := q.namePos[d.Name]; ok {
			if d.ChangeNumber >= q.rbs[p].ChangeNumber {
				q.rbs[p] = d
				q.removals[p] = removalAt(c.removals, i)
			}
			continue
		}
		q.namePos[d.Name] = len(q.rbs)
		q.rbs = append(q.rbs, d)
		q.removals = append(q.removals, removalAt(c.removals, i))
	}
	q.size = len(q.rbs)
}

// mergeKeys merges a segment change into q (same segment name). Per key the higher cn wins; at an equal cn the
// existing entry stays whatever the order of arrival (same rule as the Lua scripts in script.go).
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

// putKey stores k in the add or removal list of q. Only a higher cn replaces an existing entry; at an equal cn the
// existing entry stays, whether it is an add or a removal.
func (q *change) putKey(k segmentKey, add bool) {
	p, ok := q.pos[k.key]
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
	if k.cn <= cur.cn {
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
		q.pos[k.key] = keyPos{add: true, idx: len(q.adds) - 1}
		return
	}
	q.rems = append(q.rems, k)
	q.pos[k.key] = keyPos{add: false, idx: len(q.rems) - 1}
}

// removeKey drops the entry at p by moving the last entry of its list into the hole (order inside a change is not kept)
func (q *change) removeKey(p keyPos) {
	if p.add {
		last := len(q.adds) - 1
		if p.idx != last {
			q.adds[p.idx] = q.adds[last]
			q.pos[q.adds[p.idx].key] = keyPos{add: true, idx: p.idx}
		}
		q.adds = q.adds[:last]
		return
	}
	last := len(q.rems) - 1
	if p.idx != last {
		q.rems[p.idx] = q.rems[last]
		q.pos[q.rems[p.idx].key] = keyPos{add: false, idx: p.idx}
	}
	q.rems = q.rems[:last]
}

func (w *Writer) wake() {
	select {
	case w.signal <- struct{}{}:
	default:
	}
}

// markDegradedLocked records why Redis is behind the source. The first reason is kept. The caller holds mtx.
func (w *Writer) markDegradedLocked(reason string) {
	if !w.degraded {
		w.degraded = true
		w.degradedReason = reason
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
	if w.canResync && !w.mismatch && !w.dead && !w.fenced {
		w.stats.Resyncs++
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	w.wake()
}

// sleep waits d and reports whether the worker should try again. A Stop wakes a sleeping worker once and one immediate
// attempt follows; every later wait while draining runs min(d, time left to the drain deadline).
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
	return w.abort.Load() || (w.stopped() && !w.now().Before(w.drainUntil))
}

func (w *Writer) run(source Source) {
	defer close(w.done)
	// registered after close, so it runs first: a panic in the worker, a resync pass or a Source callback (all of them
	// run on this goroutine) is recovered and the worker exits through close(w.done)
	defer w.recoverWorker()
	// the check reads Redis, so it runs here and never blocks Start
	w.warnIfNamespaceInUse()
	// the startup purge runs after the first resync completes, or at once when there is no Source
	purgePending := true
	if source == nil {
		// a purge cut short by Stop is left to the next start; the queue is still drained
		// the purge logs its own outcome; a Stop cut is left to the next start and a mismatch is already recorded
		_ = w.startupPurge()
		purgePending = false
	}
	// dropBo paces the resync requests while a resync keeps dropping, so an outage is not hammered
	dropBo := backoff.New(2, w.opts.BackoffMaxWait)
	resyncDrops := 0
	for {
		j, ok := w.next()
		if !ok {
			return
		}
		if j.resync {
			err := w.resync(source)
			w.finishResync(err)
			if errors.Is(err, errStopped) {
				w.logger.Info("redis state: resync cut short by Stop, it reruns at the next start")
				continue
			}
			if errors.Is(err, errDropped) {
				resyncDrops++
				if resyncDrops >= maxResyncDrops {
					if resyncDrops == maxResyncDrops {
						w.logger.Error(fmt.Sprintf("redis state: %d resyncs in a row were dropped on Redis errors, stopped retrying automatically; the next successful live write, request or restart runs a new resync: %v", resyncDrops, err))
					}
					continue
				}
				if w.wait(dropBo.Next()) {
					w.RequestResync()
				}
				continue
			}
			resyncDrops = 0
			if err != nil && !errors.Is(err, ErrEnvironmentMismatch) && !errors.Is(err, ErrFenced) {
				w.logger.Error(fmt.Sprintf("redis state: resync failed, it reruns at the next request or start: %v", err))
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
				w.mtx.Lock()
				w.stats.DroppedChanges++
				w.mtx.Unlock()
				w.RequestResync()
			} else if refused {
				w.scheduleRefusedResync()
			} else if err == nil && resyncDrops >= maxResyncDrops {
				// a live write got through, so Redis is back: the capped resync retries start again
				resyncDrops = 0
				w.RequestResync()
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
	dropped := len(w.queue)
	if w.inflight != nil {
		dropped++
	}
	w.stats.DroppedChanges += int64(dropped)
	w.clearQueueLocked()
	w.inflight = nil
	w.needResync = false
	w.resyncSince = time.Time{}
	w.markDegradedLocked(w.stats.LastError)
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: writer worker panicked, the mirror stops and the proxy keeps running: %T\n%s", r, debug.Stack()))
}

// next blocks until there is something to do. A pending resync goes before queued changes.
func (w *Writer) next() (job, bool) {
	for {
		w.mtx.Lock()
		if w.aborted() || w.fenced {
			w.mtx.Unlock()
			return job{}, false
		}
		if !w.mismatch {
			if w.needResync {
				w.needResync = false
				if w.stopped() {
					// draining: a pending resync is skipped so the queue is served first
					w.resyncSince = time.Time{}
					w.mtx.Unlock()
					w.logger.Error("redis state: pending resync skipped during the shutdown drain, the queued changes are written first; Redis stays behind the source until the next start")
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

// finishResync ends the lag clock only for a resync that succeeded: after a failed or dropped one Redis is still behind
func (w *Writer) finishResync(err error) {
	w.mtx.Lock()
	if err == nil && !w.needResync {
		w.resyncSince = time.Time{}
	}
	w.mtx.Unlock()
}

// enterFenced stops the writer for good after Redis rejected a write for a lost lease: the queue is cleared, later
// changes are refused and the worker exits. Status shows Healthy=false and the fencing error as LastError.
func (w *Writer) enterFenced(last error) {
	w.mtx.Lock()
	first := !w.fenced
	w.fenced = true
	dropped := len(w.queue)
	if w.inflight != nil {
		dropped++
	}
	w.stats.DroppedChanges += int64(dropped)
	w.clearQueueLocked()
	w.inflight = nil
	w.needResync = false
	w.resyncSince = time.Time{}
	w.stats.Healthy = false
	w.stats.LastError = last.Error()
	w.mtx.Unlock()
	if first {
		w.logger.Error(fmt.Sprintf("redis state: the writer lease was lost, Redis rejected the write as fenced; the writer stopped, %d queued changes dropped: %s", dropped, last.Error()))
	}
}

// enterMismatch stops all writing: the queue is cleared and later changes are refused until Reset
func (w *Writer) enterMismatch() {
	w.mtx.Lock()
	first := !w.mismatch
	w.mismatch = true
	dropped := len(w.queue)
	if w.inflight != nil {
		dropped++
	}
	w.stats.DroppedChanges += int64(dropped)
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

// recordDrop reports a change dropped after its transient tries ran out: one Error line, unhealthy until a write succeeds
func (w *Writer) recordDrop(what string, attempts int, last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.FailedRounds++
	w.stats.LastError = last.Error()
	w.markDegradedLocked(fmt.Sprintf("%s dropped: %s", what, last.Error()))
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: %s dropped after %d attempts, Redis is behind the source until the next resync: %s", what, attempts, last.Error()))
}

// recordRefused marks the writer unhealthy after a permanent rejection. It logs nothing: the caller logs the skipped entity.
func (w *Writer) recordRefused(last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.LastError = last.Error()
	w.markDegradedLocked("a write was refused by Redis: " + last.Error())
	w.mtx.Unlock()
}

// retry runs op up to MaxAttempts times with exponential backoff between tries. It returns early with
// ErrEnvironmentMismatch (no retry), errChunkRejected (permanent bad arguments, no retry: the caller isolates the refused
// entity and reports it), errDropped once the retryable tries (transient and unknown errors alike) are exhausted (the caller
// does not isolate entities, the change is lost until a resync) or errStopped once the drain deadline has passed.
func (w *Writer) retry(what string, op func() error) error {
	bo := backoff.New(2, w.opts.BackoffMaxWait)
	var last error
	var lastClass errClass
	for attempt := 1; attempt <= w.opts.MaxAttempts; attempt++ {
		// a running resync or purge pass stops at its next Eval once draining starts
		if w.aborted() || (w.resyncPass && w.stopped()) {
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
		case errFenced:
			// not retried and not an entity refusal: the lease is gone, so the writer stops for good
			w.enterFenced(err)
			return fmt.Errorf("%w: %w", ErrFenced, err)
		case errPermanent:
			w.recordRefused(err)
			// the wrapped error keeps the Redis text for the caller's log, and errors.Is still sees errChunkRejected
			return fmt.Errorf("%w: %w", errChunkRejected, err)
		}
		last = err
		lastClass = class
		if attempt < w.opts.MaxAttempts && !w.wait(bo.Next()) {
			return errStopped
		}
	}
	// past the drain deadline the change stays in flight, so Stop counts it as dropped; before it the change is dropped
	// like any other and the worker goes on with the queue
	if w.aborted() {
		w.logger.Error(fmt.Sprintf("redis state: %s gave up after %d attempts while stopping, the change is lost and Redis stays behind the source: %s", what, w.opts.MaxAttempts, last.Error()))
		return errStopped
	}
	// the cap ends the retries, so a long outage cannot stall the queue; the resync repairs the dropped change
	w.recordDrop(what, w.opts.MaxAttempts, last)
	// the wrapped error keeps the original class and cause, so errors.Is sees both errDropped and the last Redis error
	return fmt.Errorf("%w: last error class %s: %w", errDropped, lastClass, last)
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
		removed := d.Status != statusActive || removalAt(removals, i)
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
		removed := d.Status != statusActive || removalAt(removals, i)
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
		w.reject(fmt.Sprintf("redis state: cannot serialize %q, skipped: %v", name, err))
		return entity{}, false
	}
	return entity{name: name, cn: cn, removed: removed, json: string(raw)}, true
}

// buildEntityArgs returns the ARGV of FlagScript and RuleBasedSegmentScript: fingerprint, retentionMs,
// cursor field, cursor, count, then name, cn, removed (0|1), json for every entity. The script tests run these
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
	// a live change gives every key one change number: box it once per run of equal values instead of once per key
	var boxed interface{}
	var last int64
	for _, ks := range [][]segmentKey{adds, rems} {
		for _, k := range ks {
			if boxed == nil || k.cn != last {
				boxed, last = k.cn, k.cn
			}
			args = append(args, k.key, boxed)
		}
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

// A segment group freezes as a whole on any rejected key, while flag groups track the refused names, so a hot segment's cursor stays behind until a resync.
// groupFreeze is the in-memory state of one flag or rule-based segment group. names holds the entities refused
// or skipped and not yet written cleanly. last is the last cursor sent with a real value. While names is not empty
// the group sends last, so the stored cursor stays behind the skipped entity. whole is set when the skipped set grows past MaxPendingChanges: the names are dropped and the group stays frozen until a resync releases it. Lost on restart (mirror-only gap).
type groupFreeze struct {
	names   map[string]struct{}
	last    int64
	hasLast bool
	whole   bool
}

// releaseGroup drops the skipped names and the whole-group freeze of the group that owns cursorField. A resync
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

// addSkip records a skipped entity name. Past limit names the set is discarded and the group freezes as a whole.
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

// freezeWhole freezes the whole group: later writes send the last written cursor until the group is released
func (g *groupFreeze) freezeWhole() {
	g.names = map[string]struct{}{}
	g.whole = true
}

// dropped freezes the whole group after a change was dropped: the stored cursor must not pass the lost change, and a resync releases the freeze
func (g *groupFreeze) dropped() {
	g.freezeWhole()
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
		args := append(buildEntityArgs(w.fingerprint, w.opts.Retention.Milliseconds(), cursorField, cur, part), w.opts.Owner, w.opts.Token)
		if err := w.eval(what, script, append(keys[:len(keys):len(keys)], w.keys.Lease()), args); err != nil {
			if errors.Is(err, errDropped) {
				g.dropped()
			}
			return err
		}
		g.written(part, cur)
		return nil
	}
	// skipEntity counts one refused entity; the chunk logs one line for all of its entities
	skipEntity := func(name string) {
		skipped = true
		g.addSkip(name, w.opts.MaxPendingChanges)
		w.reject("")
	}
	// refusedChunks counts the chunks in a row in which Redis took no entity; the first chunk Redis takes anything of resets it
	refusedChunks, notWritten := 0, 0
	var lastRefusal error
	for i := 0; i < n; i++ {
		if refusedChunks >= maxConsecutiveRefusals {
			// the cause is not a bad entity: stop sending, count the rest and log once
			rest := ents[i*w.opts.ChunkSize:]
			for _, e := range rest {
				g.addSkip(e.name, w.opts.MaxPendingChanges)
			}
			skipped = true
			g.freezeWhole()
			w.rejectN(len(rest), fmt.Sprintf("redis state: %s stopped after %d consecutive refused chunks, %d entities not written and the group is frozen until the next resync: %v", what, refusedChunks, notWritten+len(rest), lastRefusal))
			break
		}
		part := span(ents, i*w.opts.ChunkSize, (i+1)*w.opts.ChunkSize, 0)
		last := i == n-1
		err := send(part, last)
		if err == nil {
			refusedChunks = 0
			continue
		}
		if !errors.Is(err, errChunkRejected) {
			return refused, err
		}
		refused = true
		lastRefusal = err
		if len(part) == 0 {
			// the cursor-only call was refused: nothing can be isolated, the cursor must not move
			skipped = true
			g.freezeWhole()
			w.logger.Error(fmt.Sprintf("redis state: %s cursor update was refused by Redis, the group is frozen until the next resync: %v", what, err))
			continue
		}
		if len(part) < 2 {
			skipEntity(part[0].name)
			notWritten++
			refusedChunks++
			w.logger.Error(fmt.Sprintf("redis state: %s entity %q was refused by Redis, skipped: %v", what, part[0].name, err))
			continue
		}
		consecutive, skippedInChunk, taken, first, early := 0, 0, false, "", ""
		for j := range part {
			err := send(part[j:j+1], last && j == len(part)-1)
			if err == nil {
				consecutive = 0
				taken = true
				continue
			}
			if !errors.Is(err, errChunkRejected) {
				return refused, err
			}
			lastRefusal = err
			skipEntity(part[j].name)
			if skippedInChunk == 0 {
				first = part[j].name
			}
			skippedInChunk++
			consecutive++
			if consecutive >= maxConsecutiveRefusals && j < len(part)-1 {
				// the cause is not one bad entity: skip the rest of the chunk without calling Redis and freeze the group
				rest := part[j+1:]
				for _, e := range rest {
					g.addSkip(e.name, w.opts.MaxPendingChanges)
				}
				skipped = true
				g.freezeWhole()
				w.rejectN(len(rest), "")
				skippedInChunk += len(rest)
				early = fmt.Sprintf(", stopped isolating after %d consecutive refusals", consecutive)
				break
			}
		}
		notWritten += skippedInChunk
		if taken {
			refusedChunks = 0
		} else {
			refusedChunks++
		}
		w.logger.Error(fmt.Sprintf("redis state: %s chunk of %d entities was refused by Redis, %d skipped, the first of them %q%s: %v", what, len(part), skippedInChunk, first, early, lastRefusal))
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
		args := append(buildSegmentArgs(w.fingerprint, w.opts.Retention.Milliseconds(), name, field, cur, a, r), w.opts.Owner, w.opts.Token)
		if err := w.eval("segment write", SegmentScript, append(keys[:len(keys):len(keys)], w.keys.Lease()), args); err != nil {
			if errors.Is(err, errDropped) {
				g.dropped()
			}
			return err
		}
		g.written(nil, cur)
		return nil
	}
	// skipKey counts one refused key; the chunk logs one line for all of its keys
	skipKey := func() {
		skipped = true
		w.skipsSeen = true
		g.freezeWhole()
		w.reject("")
	}
	consecutive := 0
	// tooMany reports whether the cap is reached; the rest of the chunk is then skipped without calling Redis
	tooMany := func(rest int) bool {
		if consecutive < maxConsecutiveRefusals || rest == 0 {
			return false
		}
		skipped = true
		w.skipsSeen = true
		g.freezeWhole()
		w.rejectN(rest, "")
		return true
	}
	// refusedChunks counts the chunks in a row in which Redis took no key; the first chunk Redis takes anything of resets it
	refusedChunks, notWritten := 0, 0
	var lastRefusal error
	for i := 0; i < n; i++ {
		lo, hi := i*size, (i+1)*size
		if refusedChunks >= maxConsecutiveRefusals {
			// the cause is not a bad key: stop sending, count the rest and log once
			rest := total - lo
			skipped = true
			w.skipsSeen = true
			g.freezeWhole()
			w.rejectN(rest, fmt.Sprintf("redis state: segment %s stopped after %d consecutive refused chunks, %d keys not written and the segment is frozen until the next resync: %v", name, refusedChunks, notWritten+rest, lastRefusal))
			break
		}
		a := span(adds, lo, hi, 0)
		r := span(rems, lo, hi, len(adds))
		last := i == n-1
		err := send(a, r, last)
		if err == nil {
			refusedChunks = 0
			continue
		}
		if !errors.Is(err, errChunkRejected) {
			return refused, err
		}
		refused = true
		if len(a)+len(r) == 0 {
			// the cursor-only call was refused: nothing can be isolated, the cursor must not move
			skipped = true
			w.skipsSeen = true
			g.freezeWhole()
			w.logger.Error(fmt.Sprintf("redis state: segment %s cursor update was refused by Redis, the segment is frozen until the next resync: %v", name, err))
			continue
		}
		count := len(a) + len(r)
		lastRefusal = err
		if count < 2 {
			skipKey()
			notWritten++
			refusedChunks++
			w.logger.Error(fmt.Sprintf("redis state: segment %s chunk of %d key was refused by Redis, skipped: %v", name, count, err))
			continue
		}
		consecutive = 0
		stop, taken, skippedInChunk, early := false, false, 0, ""
		for j := 0; j < count && !stop; j++ {
			var err error
			if j < len(a) {
				err = send(a[j:j+1], nil, last && len(r) == 0 && j == len(a)-1)
			} else {
				k := j - len(a)
				err = send(nil, r[k:k+1], last && k == len(r)-1)
			}
			if err == nil {
				consecutive = 0
				taken = true
				continue
			}
			if !errors.Is(err, errChunkRejected) {
				return refused, err
			}
			lastRefusal = err
			skipKey()
			skippedInChunk++
			consecutive++
			if stop = tooMany(count - j - 1); stop {
				skippedInChunk += count - j - 1
				early = fmt.Sprintf(", stopped isolating after %d consecutive refusals", consecutive)
			}
		}
		notWritten += skippedInChunk
		if taken {
			refusedChunks = 0
		} else {
			refusedChunks++
		}
		w.logger.Error(fmt.Sprintf("redis state: segment %s chunk of %d keys was refused by Redis, %d keys skipped%s: %v", name, count, skippedInChunk, early, lastRefusal))
	}
	if skipped {
		w.logger.Error(fmt.Sprintf("redis state: segment %s skipped some data, the stored cursor was not advanced; the skipped data is not retried until the next resync", name))
	}
	return refused, nil
}
