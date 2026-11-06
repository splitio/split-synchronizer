package redisstate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/splitio/go-split-commons/v10/dtos"
)

// SegmentState is the full state of one segment as stored by the proxy
type SegmentState struct {
	Name         string
	Active       map[string]int64 // key -> change number
	Removed      map[string]int64 // key -> change number of the tombstone
	ChangeNumber int64
}

// Source gives the Writer access to the proxy's complete local state (the Bolt-backed storage).
// Implementations must read the cursor no later than the data it returns (cursor behind data is safe).
type Source interface {
	Flags() ([]dtos.SplitDTO, int64, error)                        // all flags incl. ARCHIVED, and the flag cursor
	RuleBasedSegments() ([]dtos.RuleBasedSegmentDTO, int64, error) // same for rule-based segments
	Segments(visit func(SegmentState) error) error                 // one segment at a time
}

// missingCursor is the stored cursor assumed for a segment that Redis does not know or whose cursor is unparsable
const missingCursor = int64(-2)

// resync rewrites the full local state into Redis: meta check, flags, rule-based segments, then segments one by one.
// The worker holds no mutex while it runs; every Source and client call goes through retry.
func (w *Writer) resync(source Source) error {
	fields, err := w.readMeta()
	if err != nil {
		return err
	}
	if fp, ok := fields[FingerprintField]; ok && fp != w.fingerprint {
		w.enterMismatch()
		return ErrEnvironmentMismatch
	}

	var flags []dtos.SplitDTO
	var flagsCn int64
	err = w.retry("source flags read", func() error {
		var e error
		flags, flagsCn, e = source.Flags()
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the flags from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if err != nil {
		return err
	}
	ents, skipped := w.flagEntities(flags)
	if _, err := w.writeEntities("flag resync", FlagScript, w.keys.FlagKeys(), w.keys.FlagCursorField(), ents, flagsCn, skipped); err != nil {
		return err
	}

	var rbs []dtos.RuleBasedSegmentDTO
	var rbsCn int64
	err = w.retry("source rule-based segments read", func() error {
		var e error
		rbs, rbsCn, e = source.RuleBasedSegments()
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the rule-based segments from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if err != nil {
		return err
	}
	rbsEnts, rbsSkipped := w.rbsEntities(rbs)
	if _, err := w.writeEntities("rule-based segment resync", RuleBasedSegmentScript, w.keys.RuleBasedSegmentKeys(), w.keys.RuleBasedSegmentCursorField(), rbsEnts, rbsCn, rbsSkipped); err != nil {
		return err
	}

	return w.resyncSegments(source, fields)
}

// readMeta reads the meta hash once per resync. Errors are retried like a failed script call.
func (w *Writer) readMeta() (map[string]string, error) {
	var fields map[string]string
	err := w.retry("redis meta read", func() error {
		f, e := w.client.HGetAll(w.keys.MetaUnprefixed())
		if e == nil {
			fields = f
			w.recordSuccess()
		}
		return e
	})
	return fields, err
}

// resyncSegments streams the segments from the source. A segment whose stored cursor is already at or above the
// source cursor is skipped. The first error from writing a segment stops the stream and is returned.
func (w *Writer) resyncSegments(source Source, fields map[string]string) error {
	var fatal error
	err := w.retry("source segments read", func() error {
		e := source.Segments(func(s SegmentState) error {
			if fatal != nil {
				return fatal
			}
			fatal = w.resyncSegment(s, fields)
			return fatal
		})
		if fatal != nil {
			return nil
		}
		if e != nil {
			w.logger.Error(fmt.Sprintf("redis state: reading the segments from the local state failed, retrying: %s", e.Error()))
		}
		return e
	})
	if fatal != nil {
		return fatal
	}
	return err
}

// resyncSegment writes one segment unless Redis already holds it up to its change number
func (w *Writer) resyncSegment(s SegmentState, fields map[string]string) error {
	if !validSegmentName(s.Name) {
		w.reject(fmt.Sprintf("redis state: segment name %q contains glob characters or is empty, not resynced", s.Name))
		return nil
	}
	if storedCursor(fields, w.keys.SegmentCursorField(s.Name)) >= s.ChangeNumber {
		return nil
	}
	_, err := w.writeSegment(s.Name, toSegmentKeys(s.Active), toSegmentKeys(s.Removed), s.ChangeNumber, false)
	return err
}

// storedCursor parses a cursor field of the meta hash; absent or unparsable means missingCursor
func storedCursor(fields map[string]string, field string) int64 {
	v, err := ParseCursor(fields[field])
	if err != nil {
		return missingCursor
	}
	return v
}

// toSegmentKeys converts a key map into a list sorted by key, so the calls are deterministic
func toSegmentKeys(m map[string]int64) []segmentKey {
	out := make([]segmentKey, 0, len(m))
	for k, cn := range m {
		out = append(out, segmentKey{key: k, cn: cn})
	}
	slices.SortFunc(out, func(a, b segmentKey) int { return strings.Compare(a.key, b.key) })
	return out
}
