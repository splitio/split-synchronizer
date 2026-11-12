package redisstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
)

// BootStatus classifies the redis state found at boot.
type BootStatus int

const (
	// BootAbsent means no usable state: meta is empty or holds no cursor and no segment.
	BootAbsent BootStatus = iota
	// BootForeign means the state has another fingerprint. Nothing is restored.
	BootForeign
	// BootCorrupt means the state cannot be trusted as a whole. Nothing is restored.
	BootCorrupt
	// BootUnreachable means redis could not be read after retries. Fatal at boot.
	BootUnreachable
	// BootValid means flags and rule-based segments are complete and cursors are usable.
	BootValid
)

// String returns the status name for logs.
func (s BootStatus) String() string {
	switch s {
	case BootAbsent:
		return "absent"
	case BootForeign:
		return "foreign"
	case BootCorrupt:
		return "corrupt"
	case BootUnreachable:
		return "unreachable"
	case BootValid:
		return "valid"
	default:
		return "unknown"
	}
}

// ErrUnreachable is returned by Reader.Read when redis cannot be read after retries. Fatal at boot.
var ErrUnreachable = errors.New("redis state unreachable")

// errCorrupt marks untrustworthy data. It becomes BootCorrupt or a skipped segment.
var errCorrupt = errors.New("redis state corrupt")

// ErrBudgetExhausted marks a read that ran out of its restore time budget. It always comes with errCorrupt, so the
// outcome is BootCorrupt with BudgetExhausted set.
var ErrBudgetExhausted = errors.New("restore budget exhausted")

// errTorn marks a cross-key disagreement a live writer can cause between reads of one entity group. Only this is
// retried.
var errTorn = errors.New("redis state changed while it was read")

const (
	// zscanCount is the ZSCAN page size. It only bounds each round trip.
	zscanCount = 500
	// readCallTimeout bounds each redis call. Scans have no total bound, so a progressing scan never fails.
	readCallTimeout = 30 * time.Second
	// defaultRestoreBudget bounds the whole read. A local Redis reads and restores 2M segment keys in about 2.5 s
	// (restore benchmark in the storage package), so 30 minutes leaves room for a slow or remote Redis and very
	// large segments.
	defaultRestoreBudget = 30 * time.Minute
	// defaultRetryPause is the wait before retrying a transient error or a torn read.
	defaultRetryPause = 200 * time.Millisecond
	// readAttempts is the total attempts for one redis call on transient errors.
	readAttempts = 3
	// tornReadAttempts is the total attempts (first read included) per entity group.
	tornReadAttempts = 3
	// maxExactScore (2^53): float64 cannot tell it from 2^53+1, so it and above are rejected.
	maxExactScore = 1 << 53
)

// ReadClient is the subset of redis commands the reader needs. The go-redis implementation is RedisReadClient.
type ReadClient interface {
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error)
	// HGet returns one hash field; found is false when the key or field does not exist.
	HGet(ctx context.Context, key, field string) (value string, found bool, err error)
}

// BootState is the redis state read at boot. Cursors are committed change numbers; noCursor (-1) means none stored.
// SkippedSegments lists segments that were not restored.
type BootState struct {
	Status                 BootStatus
	Reason                 string
	Flags                  []dtos.SplitDTO
	RuleBasedSegments      []dtos.RuleBasedSegmentDTO
	FlagCursor             int64
	RuleBasedSegmentCursor int64
	SegmentCursors         map[string]int64
	// Segments holds the segment payloads only for Read with a callback. ReadInto leaves it empty so a sink can
	// restore each segment and let it go.
	Segments []SegmentState
	// SegmentCount is the number of segments handed over, whichever Read form was used.
	SegmentCount    int
	SkippedSegments []string
	// SkipReasons maps each skipped segment name to why it was not restored.
	SkipReasons map[string]string
	// BudgetExhausted is set on a BootCorrupt state when the restore time budget ran out.
	BudgetExhausted bool
}

// Reader reads the redis boot state. It never writes to redis. Build it with NewReader.
type Reader struct {
	client      ReadClient
	keys        Keys
	fingerprint string
	budget      time.Duration
	retryPause  time.Duration
}

// ReaderOption changes a Reader default. The options are unexported: only tests change the defaults.
type ReaderOption func(*readerConfig)

type readerConfig struct {
	budget     time.Duration
	retryPause time.Duration
}

func withRestoreBudget(d time.Duration) ReaderOption {
	return func(c *readerConfig) { c.budget = d }
}

func withRetryPause(d time.Duration) ReaderOption {
	return func(c *readerConfig) { c.retryPause = d }
}

// NewReader returns a Reader that accepts only state written with the given fingerprint.
func NewReader(client ReadClient, keys Keys, fingerprint string, opts ...ReaderOption) (*Reader, error) {
	if client == nil {
		return nil, errors.New("redis state reader needs a read client")
	}
	if fingerprint == "" {
		return nil, errors.New("redis state reader needs a fingerprint")
	}
	cfg := readerConfig{budget: defaultRestoreBudget, retryPause: defaultRetryPause}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Reader{client: client, keys: keys, fingerprint: fingerprint, budget: cfg.budget, retryPause: cfg.retryPause}, nil
}

// RestoreSink receives a valid state as it is read. Entities is called once, after the flags and rule-based segments
// are read and validated and before any segment. Segment is then called once per restorable segment, and the reader
// keeps no reference to the payload afterwards.
type RestoreSink interface {
	Entities(flags []dtos.SplitDTO, ruleBasedSegments []dtos.RuleBasedSegmentDTO) error
	Segment(SegmentState) error
}

// visitSink adapts a segment callback to RestoreSink; it ignores the entities.
type visitSink struct{ visit func(SegmentState) error }

func (v visitSink) Entities([]dtos.SplitDTO, []dtos.RuleBasedSegmentDTO) error { return nil }

func (v visitSink) Segment(s SegmentState) error {
	if v.visit == nil {
		return nil
	}
	return v.visit(s)
}

// Read classifies the state and, when valid, calls visit once per segment after reading meta (the cursors). Every
// visited segment is also kept in BootState.Segments. Use ReadInto to stream without keeping the payloads.
// Corrupt, foreign and absent state return a nil error. Unreachable redis returns BootUnreachable and ErrUnreachable.
// A visit error is returned wrapped. Check err first: BootState is meaningful on error only for BootUnreachable.
func (r *Reader) Read(ctx context.Context, visit func(SegmentState) error) (BootState, error) {
	return r.readWith(ctx, visitSink{visit: visit}, true)
}

// ReadInto is Read for a RestoreSink. BootState.Segments stays empty; SegmentCount and SegmentCursors are filled.
// A sink error is returned wrapped and is not a corrupt state, so the caller must treat it as fatal. When the outcome
// is not BootValid, or the read fails after some segments were handed over, the caller must discard what the sink
// wrote.
func (r *Reader) ReadInto(ctx context.Context, sink RestoreSink) (BootState, error) {
	return r.readWith(ctx, sink, false)
}

func (r *Reader) readWith(ctx context.Context, sink RestoreSink, retain bool) (BootState, error) {
	budgetCtx, cancel := context.WithTimeout(ctx, r.budget)
	defer cancel()
	state, err := r.read(budgetCtx, sink, retain)
	switch {
	case err == nil:
		return state, nil
	case errors.Is(err, errCorrupt):
		return BootState{Status: BootCorrupt, Reason: err.Error(), BudgetExhausted: errors.Is(err, ErrBudgetExhausted), FlagCursor: noCursor, RuleBasedSegmentCursor: noCursor}, nil
	case errors.Is(err, ErrUnreachable):
		return BootState{Status: BootUnreachable, Reason: err.Error(), FlagCursor: noCursor, RuleBasedSegmentCursor: noCursor}, err
	default:
		return BootState{}, err
	}
}

func (r *Reader) read(ctx context.Context, sink RestoreSink, retain bool) (BootState, error) {
	meta, err := readCall(ctx, r.retryPause, "meta", func(c context.Context) (map[string]string, error) {
		return r.client.HGetAll(c, r.keys.Meta())
	})
	if err != nil {
		return BootState{}, err
	}
	if len(meta) == 0 {
		return BootState{Status: BootAbsent, Reason: "no redis state", FlagCursor: noCursor, RuleBasedSegmentCursor: noCursor}, nil
	}
	fingerprint, ok := meta[FingerprintField]
	if !ok {
		return BootState{}, corruptf("meta has no fingerprint")
	}
	if fingerprint != r.fingerprint {
		return BootState{Status: BootForeign, Reason: "fingerprint differs", FlagCursor: noCursor, RuleBasedSegmentCursor: noCursor}, nil
	}

	// The cursors are read before the entity data on purpose: a write landing in between leaves cursor <= data, which
	// only re-syncs a little extra and never skips a change.
	flagCursor, err := optionalCursor(meta, r.keys.FlagCursorField())
	if err != nil {
		return BootState{}, err
	}
	rbsCursor, err := optionalCursor(meta, r.keys.RuleBasedSegmentCursorField())
	if err != nil {
		return BootState{}, err
	}
	names := segmentNamesOf(meta)
	if flagCursor == noCursor && rbsCursor == noCursor && len(names) == 0 {
		return BootState{Status: BootAbsent, Reason: "no cursors stored", FlagCursor: noCursor, RuleBasedSegmentCursor: noCursor}, nil
	}

	var flags []dtos.SplitDTO
	err = r.retryTorn(ctx, func() error {
		var err error
		flags, err = readEntities(ctx, r, "flags", r.keys.FlagData(), r.keys.FlagIndex(), r.keys.FlagRemoved(),
			DecodeFlag, flagChangeNumber, func(d dtos.SplitDTO) (string, string) { return d.Name, d.Status })
		return err
	})
	if err != nil {
		return BootState{}, err
	}
	if flagCursor == noCursor && len(flags) > 0 {
		return BootState{}, corruptf("flags stored without a cursor")
	}

	var rbs []dtos.RuleBasedSegmentDTO
	err = r.retryTorn(ctx, func() error {
		var err error
		rbs, err = readEntities(ctx, r, "rule-based segments", r.keys.RuleBasedSegmentData(), r.keys.RuleBasedSegmentIndex(), r.keys.RuleBasedSegmentRemoved(),
			DecodeRuleBasedSegment, rbsChangeNumber, func(d dtos.RuleBasedSegmentDTO) (string, string) { return d.Name, d.Status })
		return err
	})
	if err != nil {
		return BootState{}, err
	}
	if rbsCursor == noCursor && len(rbs) > 0 {
		return BootState{}, corruptf("rule-based segments stored without a cursor")
	}

	if err := sink.Entities(flags, rbs); err != nil {
		return BootState{}, fmt.Errorf("restoring flags and rule-based segments: %w", err)
	}
	state := BootState{
		Status:                 BootValid,
		Reason:                 "valid",
		Flags:                  flags,
		RuleBasedSegments:      rbs,
		FlagCursor:             flagCursor,
		RuleBasedSegmentCursor: rbsCursor,
		SegmentCursors:         map[string]int64{},
	}
	inMeta := map[string]bool{}
	for _, name := range names {
		inMeta[name] = true
	}
	for _, name := range referencedSegmentNames(flags, rbs) {
		if !inMeta[name] {
			state.skipSegment(name, "referenced by a flag or rule-based segment but has no cursor in meta")
			continue
		}
		if err := r.readSegment(ctx, meta, name, sink, retain, &state); err != nil {
			return BootState{}, err
		}
	}
	return state, nil
}

// skipSegment records a segment that was not restored and why.
func (s *BootState) skipSegment(name, reason string) {
	s.SkippedSegments = append(s.SkippedSegments, name)
	if s.SkipReasons == nil {
		s.SkipReasons = map[string]string{}
	}
	s.SkipReasons[name] = reason
}

// retryTorn repeats an entity group read on errTorn only, up to tornReadAttempts. Other errors return at once.
func (r *Reader) retryTorn(ctx context.Context, read func() error) error {
	var err error
	for attempt := 1; attempt <= tornReadAttempts; attempt++ {
		if err = read(); !errors.Is(err, errTorn) {
			return err
		}
		if attempt == tornReadAttempts {
			break
		}
		timer := time.NewTimer(r.retryPause)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.Canceled) {
				return fmt.Errorf("%w: retrying a torn read: %w", ErrUnreachable, ctx.Err())
			}
			return budgetf("while retrying a torn read: %v", err)
		case <-timer.C:
		}
	}
	return err
}

// readSegment visits one segment, or records it in SkippedSegments when it cannot be trusted. A segment whose cursor
// is -1 is held by a full replace in progress or was never committed, so it is skipped whatever its sets hold.
// The cursor is read again after both scans; a change means a writer touched the segment meanwhile (torn read:
// retried, then skipped).
func (r *Reader) readSegment(ctx context.Context, meta map[string]string, name string, sink RestoreSink, retain bool, state *BootState) error {
	skip := func(reason string) { state.skipSegment(name, reason) }
	field := r.keys.SegmentCursorField(name)
	cursor, err := storedCursorOf(meta, field)
	if err != nil {
		skip(err.Error())
		return nil
	}
	var seg SegmentState
	var held bool
	err = r.retryTorn(ctx, func() error {
		if cursor == noCursor {
			held = true
			return nil
		}
		active, err := r.scanScores(ctx, r.keys.SegmentActive(name), "segment")
		if err != nil {
			return err
		}
		removed, err := r.scanScores(ctx, r.keys.SegmentTombstones(name), "segment tombstones")
		if err != nil {
			return err
		}
		current, err := r.currentSegmentCursor(ctx, field)
		if err != nil {
			return err
		}
		if current != cursor {
			cursor = current
			return tornf("segment %s cursor changed while it was read", name)
		}
		resolveBothSets(active, removed)
		seg = SegmentState{Name: name, Active: active, Removed: removed, ChangeNumber: cursor}
		return nil
	})
	if err != nil {
		if errors.Is(err, errCorrupt) && ctx.Err() == nil {
			skip(err.Error())
			return nil
		}
		return err
	}
	if held {
		skip("segment cursor is -1: a full replace is in progress or nothing was committed")
		return nil
	}
	if err := sink.Segment(seg); err != nil {
		return fmt.Errorf("restoring segment %s: %w", name, err)
	}
	if retain {
		state.Segments = append(state.Segments, seg)
	}
	state.SegmentCount++
	state.SegmentCursors[name] = cursor
	return nil
}

// currentSegmentCursor reads a segment cursor straight from meta, to detect a writer that moved it during the scans.
func (r *Reader) currentSegmentCursor(ctx context.Context, field string) (int64, error) {
	type hashField struct {
		value string
		found bool
	}
	got, err := readCall(ctx, r.retryPause, "segment cursor", func(c context.Context) (hashField, error) {
		v, found, err := r.client.HGet(c, r.keys.Meta(), field)
		return hashField{value: v, found: found}, err
	})
	if err != nil {
		return 0, err
	}
	if !got.found {
		// SegmentDropScript removes the field, so a live writer can cause this: torn, not corrupt.
		return 0, tornf("cursor %s disappeared while the segment was read", field)
	}
	return storedCursorOf(map[string]string{field: got.value}, field)
}

// segmentNamesOf returns the sorted segment names registered in meta. The writer sets a seg.<name>.till field
// atomically with every segment key, so meta is the complete registry.
func segmentNamesOf(meta map[string]string) []string {
	var names []string
	for field := range meta {
		if name, ok := segmentNameOfCursorField(field); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// referencedSegmentNames keeps the names that a restored flag or rule-based segment references: user-defined matchers
// in conditions, and standard excluded segments of rule-based segments. Unreferenced leftovers are not read or
// restored. Removed (non-ACTIVE) entities keep their stored conditions but are not served, so they reference nothing.
func referencedSegmentNames(flags []dtos.SplitDTO, rbs []dtos.RuleBasedSegmentDTO) []string {
	referenced := map[string]bool{}
	for _, f := range flags {
		if f.Status != statusActive {
			continue
		}
		for _, c := range f.Conditions {
			addMatcherReferences(referenced, c.MatcherGroup.Matchers)
		}
	}
	for _, rb := range rbs {
		if rb.Status != statusActive {
			continue
		}
		for _, c := range rb.Conditions {
			addMatcherReferences(referenced, c.MatcherGroup.Matchers)
		}
		for _, ex := range rb.Excluded.Segments {
			if ex.Type == dtos.TypeStandard {
				referenced[ex.Name] = true
			}
		}
	}
	out := make([]string, 0, len(referenced))
	for name := range referenced {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// addMatcherReferences marks the segments named by the user-defined segment matchers.
func addMatcherReferences(referenced map[string]bool, matchers []dtos.MatcherDTO) {
	for _, m := range matchers {
		if m.UserDefinedSegment != nil {
			referenced[m.UserDefinedSegment.SegmentName] = true
		}
	}
}

// resolveBothSets drops the stale side of a key present in both sets, which a writer moving it between two ZSCANs can
// cause. The higher change number wins; a tie is a removal.
func resolveBothSets(active, removed map[string]int64) {
	for key, removedAt := range removed {
		activeAt, both := active[key]
		if !both {
			continue
		}
		if removedAt >= activeAt {
			delete(active, key)
		} else {
			delete(removed, key)
		}
	}
}

func flagChangeNumber(data []byte) (int64, error) {
	var d dtos.SplitDTO
	err := json.Unmarshal(data, &d)
	return d.ChangeNumber, err
}

func rbsChangeNumber(data []byte) (int64, error) {
	var d dtos.RuleBasedSegmentDTO
	err := json.Unmarshal(data, &d)
	return d.ChangeNumber, err
}

// readEntities rebuilds flags or rule-based segments from the data hash, the index and the removed set. For a
// non-removed entity the JSON change number must equal the index score. Cross-read disagreements are errTorn.
func readEntities[T any](ctx context.Context, r *Reader, label, dataKey, indexKey, removedKey string,
	decode func([]byte, int64, bool) (T, error),
	changeNumberOf func([]byte) (int64, error),
	identity func(T) (string, string),
) ([]T, error) {
	data, err := readCall(ctx, r.retryPause, label+" data", func(c context.Context) (map[string]string, error) {
		return r.client.HGetAll(c, dataKey)
	})
	if err != nil {
		return nil, err
	}
	index, err := r.scanScores(ctx, indexKey, label+" index")
	if err != nil {
		return nil, err
	}
	removed, err := r.scanScores(ctx, removedKey, label+" removed")
	if err != nil {
		return nil, err
	}
	if len(data) != len(index) {
		return nil, tornf("%s data and index differ", label)
	}
	for name := range removed {
		if _, ok := index[name]; !ok {
			return nil, tornf("%s removed entry is not indexed", label)
		}
	}

	out := make([]T, 0, len(data))
	for _, name := range slices.Sorted(maps.Keys(data)) {
		cn, ok := index[name]
		if !ok {
			return nil, tornf("%s data has an entry missing from the index", label)
		}
		_, isRemoved := removed[name]
		raw := []byte(data[name])
		if !isRemoved {
			jsonCN, cnErr := changeNumberOf(raw)
			if cnErr != nil {
				return nil, corruptf("%s entry does not decode: %w", label, cnErr)
			}
			if jsonCN != cn {
				return nil, tornf("%s entry change number differs from its index score", label)
			}
		}
		entity, err := decode(raw, cn, isRemoved)
		if err != nil {
			return nil, corruptf("%s entry does not decode: %w", label, err)
		}
		entityName, status := identity(entity)
		if name == "" || entityName != name {
			return nil, corruptf("%s entry name does not match its key", label)
		}
		if !isRemoved && status != statusActive {
			return nil, corruptf("%s entry is not removed but its status is not ACTIVE", label)
		}
		out = append(out, entity)
	}
	return out, nil
}

// scanScores reads a sorted set with ZSCAN in zscanCount pages. Members repeated across pages are collapsed.
// Non-integer scores are errCorrupt.
func (r *Reader) scanScores(ctx context.Context, key, label string) (map[string]int64, error) {
	type zscanPage struct {
		members []string
		next    uint64
	}
	out := map[string]int64{}
	seenCursors := map[uint64]bool{}
	var cursor uint64
	for {
		page, err := readCall(ctx, r.retryPause, label, func(c context.Context) (zscanPage, error) {
			members, next, err := r.client.ZScan(c, key, cursor, zscanCount)
			return zscanPage{members: members, next: next}, err
		})
		if err != nil {
			return nil, err
		}
		if len(page.members)%2 != 0 {
			return nil, corruptf("%s scan page has an odd number of values", label)
		}
		for i := 0; i < len(page.members); i += 2 {
			score, err := strconv.ParseFloat(page.members[i+1], 64)
			if err != nil || score != math.Trunc(score) || math.Abs(score) >= maxExactScore {
				return nil, corruptf("%s score is not an exact integer", label)
			}
			if score < 0 {
				return nil, corruptf("%s score is negative", label)
			}
			out[page.members[i]] = int64(score)
		}
		if page.next == 0 {
			return out, nil
		}
		if seenCursors[page.next] {
			return nil, corruptf("%s scan returned the cursor %d twice", label, page.next)
		}
		seenCursors[page.next] = true
		cursor = page.next
	}
}

// readCall runs one redis call, retrying transient errors up to readAttempts times with pause between attempts.
// WRONGTYPE is corrupt and not retried. A connection error or a cancelled caller context is ErrUnreachable. Only a
// spent restore budget is corrupt: a timeout after it, or a retry pause it cuts short, because the read was too slow.
func readCall[T any](ctx context.Context, pause time.Duration, what string, call func(context.Context) (T, error)) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		out, err := callWithRetry(ctx, readCallTimeout, call)
		if err == nil {
			return out, nil
		}
		if strings.HasPrefix(err.Error(), "WRONGTYPE") {
			return zero, corruptf("%s has the wrong redis type", what)
		}
		if !isTransient(err) || attempt >= readAttempts || ctx.Err() != nil {
			if ctx.Err() != nil && !errors.Is(ctx.Err(), context.Canceled) && isTimeout(err) {
				return zero, budgetf("while reading %s", what)
			}
			return zero, fmt.Errorf("%w: reading %s: %w", ErrUnreachable, what, err)
		}
		timer := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.Canceled) {
				return zero, fmt.Errorf("%w: reading %s: %w", ErrUnreachable, what, ctx.Err())
			}
			return zero, budgetf("while reading %s", what)
		case <-timer.C:
		}
	}
}

// callWithRetry runs call under a fresh deadline and retries once on timeout while ctx is still live.
func callWithRetry[T any](ctx context.Context, timeout time.Duration, call func(context.Context) (T, error)) (T, error) {
	attempt := func() (T, error) {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return call(callCtx)
	}
	out, err := attempt()
	if err != nil && isTimeout(err) && ctx.Err() == nil {
		return attempt()
	}
	return out, err
}

// isTimeout reports whether err is a context deadline or a network timeout.
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}

// isTransient reports whether a retry can clear err: connection reset, I/O timeout, or LOADING, MOVED, ASK, TRYAGAIN.
func isTransient(err error) bool {
	msg := err.Error()
	for _, s := range []string{"connection reset", "connection refused", "broken pipe", "i/o timeout", "LOADING", "TRYAGAIN"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return msg == "EOF" || strings.HasSuffix(msg, ": EOF") || strings.HasPrefix(msg, "MOVED ") || strings.HasPrefix(msg, "ASK ")
}

// optionalCursor returns the stored cursor, or noCursor when the field is absent.
func optionalCursor(meta map[string]string, field string) (int64, error) {
	if _, ok := meta[field]; !ok {
		return noCursor, nil
	}
	return storedCursorOf(meta, field)
}

// storedCursorOf parses a cursor field of meta. The caller decides whether an error corrupts the whole state or one
// segment.
func storedCursorOf(meta map[string]string, field string) (int64, error) {
	raw, ok := meta[field]
	if !ok {
		return 0, corruptf("cursor %s is missing", field)
	}
	cursor, err := ParseCursor(raw)
	if err != nil {
		return 0, corruptf("cursor %s does not parse: value %q is not an integer", field, truncateForLog(raw))
	}
	if cursor < noCursor {
		return 0, corruptf("cursor %s is below the no-cursor value", field)
	}
	return cursor, nil
}

// truncateForLog caps a raw stored value before it is embedded in a log-bound reason.
// It cuts by bytes, so a multi-byte rune at the cut may be split; that is acceptable for logs.
func truncateForLog(s string) string {
	if len(s) <= 32 {
		return s
	}
	return s[:32] + "..."
}

// tornf is corruptf for a cross-key disagreement that a retry may clear (see errTorn).
func tornf(format string, args ...any) error {
	return tornError{corruptf(format, args...)}
}

// tornError is errCorrupt that also matches errTorn, with the same message.
type tornError struct{ error }

func (e tornError) Unwrap() error        { return e.error }
func (e tornError) Is(target error) bool { return target == errTorn }

func budgetf(format string, args ...any) error {
	return fmt.Errorf("%w: %w: "+format, append([]any{errCorrupt, ErrBudgetExhausted}, args...)...)
}

func corruptf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errCorrupt}, args...)...)
}
