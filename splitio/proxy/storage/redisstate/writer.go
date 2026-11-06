package redisstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/backoff"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
)

const (
	statusActive = "ACTIVE"

	defaultRetention         = 168 * time.Hour
	defaultMaxAttempts       = 10
	defaultBackoffMaxWait    = 60 * time.Second
	defaultMaxPendingChanges = 3000000
	defaultChunkSize         = 1000

	// noCursor is the cursor argument that makes a script leave the stored cursor untouched
	noCursor = int64(-1)
)

var (
	// ErrEnvironmentMismatch is returned when Redis holds state of another environment/configuration
	ErrEnvironmentMismatch = errors.New("redis state belongs to a different environment or configuration")
	// ErrInvalidOptions is returned when the writer is built with an invalid option or dependency
	ErrInvalidOptions = errors.New("invalid redis state writer options")

	errStopped       = errors.New("redis state writer stopped")
	errChunkRejected = errors.New("redis state chunk rejected by the script")
)

// Client is the subset of the go-toolkit prefixed Redis client used by the Writer
type Client interface {
	Eval(script string, keys []string, args ...interface{}) error
	// HGetAll reads a hash. The prefixed client adds Prefix()+"." itself, so pass Keys.MetaUnprefixed()
	HGetAll(key string) (map[string]string, error)
	Prefix() string
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
}

// Options configures a Writer
type Options struct {
	Retention         time.Duration // how long removals are kept, default 168h
	MaxAttempts       int           // attempts per round, default 10
	BackoffMaxWait    time.Duration // default 60s
	MaxPendingChanges int           // default 3000000 (entities + keys queued; a queued key is 24 bytes, about 72 MB at the cap)
	ChunkSize         int           // default 1000 (entities or keys per script call)
}

// DefaultOptions returns the default writer options
func DefaultOptions() Options {
	return Options{
		Retention:         defaultRetention,
		MaxAttempts:       defaultMaxAttempts,
		BackoffMaxWait:    defaultBackoffMaxWait,
		MaxPendingChanges: defaultMaxPendingChanges,
		ChunkSize:         defaultChunkSize,
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
	segment    string
	adds       []segmentKey
	rems       []segmentKey
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
	client      Client
	keys        Keys
	fingerprint string
	opts        Options
	logger      logging.LoggerInterface

	now     func() time.Time
	wait    func(time.Duration) bool
	marshal func(v interface{}) ([]byte, error)

	signal   chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	mtx           sync.Mutex
	started       bool
	queue         []*change
	queuedSize    int
	inflight      *change
	needResync    bool
	canResync     bool
	resyncSince   time.Time
	mismatch      bool
	refusedLogged bool
	stats         Status
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
	case opts.ChunkSize <= 0:
		return nil, fmt.Errorf("%w: chunk size must be positive", ErrInvalidOptions)
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

// Start launches the single background worker. The worker first calls source.Resync (if source != nil), then drains the queue.
func (w *Writer) Start(source Source) {
	w.mtx.Lock()
	if w.started {
		w.mtx.Unlock()
		w.logger.Warning("redis state writer already started")
		return
	}
	w.started = true
	w.canResync = source != nil
	if source != nil {
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	go w.run(source)
}

// Stop signals the worker and returns after it exits. Pending changes are NOT flushed (the startup resync reconciles them next run).
func (w *Writer) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
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
	}
	w.mtx.Unlock()
	w.logger.Info(fmt.Sprintf("redis state writer stopped, %d queued changes dropped (the next startup resync recovers them)", dropped))
}

// Reset deletes every key of this environment namespace and clears the mismatch flag. Call before Start, only when Initialization.ForceFreshStartup is set.
func (w *Writer) Reset() error {
	if err := w.client.Eval(ResetScript, nil, w.keys.Pattern()); err != nil {
		return fmt.Errorf("deleting the redis state of environment %s: %w", w.keys.Env(), err)
	}
	w.mtx.Lock()
	w.mismatch = false
	w.refusedLogged = false
	w.mtx.Unlock()
	return nil
}

// PutFlags enqueues a feature flag change. It never blocks and never calls Redis.
func (w *Writer) PutFlags(toAdd, toRemove []dtos.SplitDTO, cn int64) {
	if w.refuse() {
		return
	}
	list := make([]dtos.SplitDTO, 0, len(toAdd)+len(toRemove))
	list = append(append(list, toAdd...), toRemove...)
	w.enqueue(&change{kind: kindFlags, cn: cn, flags: list, size: len(list)})
}

// PutRuleBasedSegments enqueues a rule-based segment change. It never blocks and never calls Redis.
func (w *Writer) PutRuleBasedSegments(toAdd, toRemove []dtos.RuleBasedSegmentDTO, cn int64) {
	if w.refuse() {
		return
	}
	list := make([]dtos.RuleBasedSegmentDTO, 0, len(toAdd)+len(toRemove))
	list = append(append(list, toAdd...), toRemove...)
	w.enqueue(&change{kind: kindRuleBasedSegments, cn: cn, rbs: list, size: len(list)})
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

func validSegmentName(name string) bool {
	return name != "" && !strings.ContainsAny(name, globChars)
}

// refuse counts and reports a change refused because of an environment mismatch
func (w *Writer) refuse() bool {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if !w.mismatch {
		return false
	}
	w.stats.RefusedChanges++
	if !w.refusedLogged {
		w.refusedLogged = true
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

	w.mtx.Lock()
	if w.mismatch {
		w.stats.RefusedChanges++
		w.mtx.Unlock()
		return
	}
	if len(w.queue) > 0 && w.queuedSize+c.size > w.opts.MaxPendingChanges {
		dropped := len(w.queue)
		w.queue = nil
		w.queuedSize = 0
		w.stats.Resyncs++
		w.scheduleResyncLocked()
		outcome := "a full resync from the local state is scheduled"
		if !w.canResync {
			outcome = "no resync is scheduled because no source is configured"
		}
		w.mtx.Unlock()
		w.logger.Error(fmt.Sprintf("redis state: more than %d changes pending, %d queued changes dropped and %s", w.opts.MaxPendingChanges, dropped, outcome))
		w.wake()
		return
	}
	w.queue = append(w.queue, c)
	w.queuedSize += c.size
	w.mtx.Unlock()
	w.wake()
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
	if w.canResync {
		w.stats.Resyncs++
		w.scheduleResyncLocked()
	}
	w.mtx.Unlock()
	w.wake()
}

func (w *Writer) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-w.stop:
		return false
	}
}

func (w *Writer) stopped() bool {
	select {
	case <-w.stop:
		return true
	default:
		return false
	}
}

func (w *Writer) run(source Source) {
	defer close(w.done)
	for {
		j, ok := w.next()
		if !ok {
			return
		}
		var err error
		if j.resync {
			err = w.resync(source)
			w.finishResync()
		} else {
			var refused bool
			refused, err = w.write(j.change)
			w.finishChange()
			if refused {
				w.scheduleRefusedResync()
			}
		}
		if errors.Is(err, errStopped) {
			return
		}
	}
}

// next blocks until there is something to do. A pending resync goes before queued changes.
func (w *Writer) next() (job, bool) {
	for {
		w.mtx.Lock()
		if w.stopped() {
			w.mtx.Unlock()
			return job{}, false
		}
		if !w.mismatch {
			if w.needResync {
				w.needResync = false
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
	w.queue = nil
	w.queuedSize = 0
	w.inflight = nil
	w.needResync = false
	w.resyncSince = time.Time{}
	w.mtx.Unlock()
	if first {
		w.logger.Error(fmt.Sprintf("redis state: the state in Redis belongs to a different environment or configuration (environment %s); nothing will be written, %d queued changes dropped. Use a different namespace or start with a forced fresh startup", w.keys.Env(), dropped))
	}
}

func (w *Writer) recordSuccess() {
	w.mtx.Lock()
	w.stats.Healthy = true
	w.stats.LastSuccessUnixMs = w.now().UnixMilli()
	w.mtx.Unlock()
}

func (w *Writer) recordFailedRound(what string, attempts int, last error) {
	w.mtx.Lock()
	w.stats.Healthy = false
	w.stats.FailedRounds++
	w.stats.LastError = last.Error()
	w.mtx.Unlock()
	w.logger.Error(fmt.Sprintf("redis state: %s failed %d times in a row, retrying: %s", what, attempts, last.Error()))
}

// retry runs op in rounds of MaxAttempts tries with exponential backoff between tries. A failed round is reported and
// a new one starts after BackoffMaxWait, forever: a change is never dropped. It returns early with
// ErrEnvironmentMismatch (no retry), errChunkRejected (bad arguments, no retry: the caller isolates the refused
// entity and reports it) or errStopped.
func (w *Writer) retry(what string, op func() error) error {
	for {
		bo := backoff.New(2, w.opts.BackoffMaxWait)
		var last error
		for attempt := 1; attempt <= w.opts.MaxAttempts; attempt++ {
			if w.stopped() {
				return errStopped
			}
			err := op()
			if err == nil {
				return nil
			}
			if ErrorHasMarker(err, EnvMismatchMarker) {
				w.enterMismatch()
				return ErrEnvironmentMismatch
			}
			if ErrorHasMarker(err, BadArgsMarker) {
				return errChunkRejected
			}
			last = err
			if attempt < w.opts.MaxAttempts && !w.wait(bo.Next()) {
				return errStopped
			}
		}
		w.recordFailedRound(what, w.opts.MaxAttempts, last)
		if !w.wait(w.opts.BackoffMaxWait) {
			return errStopped
		}
	}
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
		ents, skipped := w.flagEntities(c.flags)
		return w.writeEntities("flag write", FlagScript, w.keys.FlagKeys(), w.keys.FlagCursorField(), ents, c.cn, skipped)
	case kindRuleBasedSegments:
		ents, skipped := w.rbsEntities(c.rbs)
		return w.writeEntities("rule-based segment write", RuleBasedSegmentScript, w.keys.RuleBasedSegmentKeys(), w.keys.RuleBasedSegmentCursorField(), ents, c.cn, skipped)
	default:
		return w.writeSegment(c.segment, c.adds, c.rems, c.cn, false)
	}
}

// flagEntities serializes feature flags. The removed flag comes from the DTO status, never from the list it arrived in.
func (w *Writer) flagEntities(list []dtos.SplitDTO) ([]entity, bool) {
	ents := make([]entity, 0, len(list))
	skipped := false
	for i := range list {
		d := &list[i]
		e, ok := w.newEntity(d.Name, d.ChangeNumber, d.Status, *d)
		ents, skipped = appendEntity(ents, e, ok, skipped)
	}
	return ents, skipped
}

// rbsEntities serializes rule-based segments with the same rules as flagEntities
func (w *Writer) rbsEntities(list []dtos.RuleBasedSegmentDTO) ([]entity, bool) {
	ents := make([]entity, 0, len(list))
	skipped := false
	for i := range list {
		d := &list[i]
		e, ok := w.newEntity(d.Name, d.ChangeNumber, d.Status, *d)
		ents, skipped = appendEntity(ents, e, ok, skipped)
	}
	return ents, skipped
}

func appendEntity(ents []entity, e entity, ok, skipped bool) ([]entity, bool) {
	if !ok {
		return ents, true
	}
	return append(ents, e), skipped
}

// newEntity serializes a DTO. The removed flag comes from the DTO status, never from the list it arrived in.
func (w *Writer) newEntity(name string, cn int64, status string, dto interface{}) (entity, bool) {
	raw, err := w.marshal(dto)
	if err != nil {
		w.reject(fmt.Sprintf("redis state: cannot serialize %q, skipped: %s", name, err.Error()))
		return entity{}, false
	}
	return entity{name: name, cn: cn, removed: status != statusActive, json: string(raw)}, true
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

// writeEntities writes flags or rule-based segments in chunks. Only the last call carries the real cursor, and
// not even that one when something was skipped, so the stored cursor is never ahead of the stored data.
// A chunk refused with PROXY_BAD_ARGS is retried one entity at a time, so only the refused entity is skipped.
func (w *Writer) writeEntities(what, script string, keys []string, cursorField string, ents []entity, cursor int64, skipped bool) (bool, error) {
	n := chunkCount(len(ents), w.opts.ChunkSize)
	refused := false
	send := func(part []entity, last bool) error {
		cur := noCursor
		if last && !skipped {
			cur = cursor
		}
		args := []interface{}{w.fingerprint, w.now().UnixMilli(), w.opts.Retention.Milliseconds(), cursorField, cur, len(part)}
		for _, e := range part {
			removed := "0"
			if e.removed {
				removed = "1"
			}
			args = append(args, e.name, e.cn, removed, e.json)
		}
		return w.eval(what, script, keys, args)
	}
	skipEntity := func(name string) {
		skipped = true
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
	send := func(a, r []segmentKey, last bool) error {
		cur := noCursor
		if last && !skipped {
			cur = cursor
		}
		args := make([]interface{}, 0, 8+2*(len(a)+len(r)))
		args = append(args, w.fingerprint, w.now().UnixMilli(), w.opts.Retention.Milliseconds(), name, field, cur, len(a), len(r))
		for _, k := range a {
			args = append(args, k.key, k.cn)
		}
		for _, k := range r {
			args = append(args, k.key, k.cn)
		}
		return w.eval("segment write", SegmentScript, keys, args)
	}
	skipKey := func(k segmentKey) {
		skipped = true
		w.reject(fmt.Sprintf("redis state: segment %s key %q was rejected by the script because of inconsistent arguments, skipped", name, k.key))
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
