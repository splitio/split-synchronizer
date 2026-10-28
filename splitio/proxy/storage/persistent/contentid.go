package persistent

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"slices"
	"sort"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"

	bolt "go.etcd.io/bbolt" // new fork maintained by etcd
)

// contentIDVersion is hashed first. Bump it when the encoding or the hashed fields change.
const contentIDVersion = "splitio-proxy/content-id/v1"

// Section tags framing each part of the content identifier
const (
	contentIDSectionSpecVersion = "flagSpecVersion"
	contentIDSectionFlagSets    = "flagSets"
	contentIDSectionFlags       = "flags"
	contentIDSectionRBSegments  = "ruleBasedSegments"
	contentIDSectionSegments    = "segments"
)

// contentEntry is a logical item (flag or rule-based segment) reduced to its identity
type contentEntry struct {
	name         string
	changeNumber int64
	canonical    []byte
}

// contentWriter feeds unambiguously framed values into a hash
type contentWriter struct {
	h   hash.Hash
	buf [8]byte
}

func (w *contentWriter) writeUint64(v uint64) {
	binary.BigEndian.PutUint64(w.buf[:], v)
	w.h.Write(w.buf[:])
}

func (w *contentWriter) writeCount(n int) {
	w.writeUint64(uint64(n))
}

func (w *contentWriter) writeInt64(v int64) {
	w.writeUint64(uint64(v))
}

func (w *contentWriter) writeBytes(b []byte) {
	w.writeUint64(uint64(len(b)))
	w.h.Write(b)
}

func (w *contentWriter) writeString(s string) {
	w.writeBytes([]byte(s))
}

// activeChangesEntries returns the ACTIVE items of a changes collection, decoded into their DTO form.
// A missing bucket is treated as an empty collection.
func activeChangesEntries[T any](tx *bolt.Tx, collection string) ([]contentEntry, error) {
	bucket := tx.Bucket([]byte(collection))
	if bucket == nil {
		return nil, nil
	}

	var entries []contentEntry
	err := bucket.ForEach(func(k, v []byte) error {
		var item ChangesItem
		if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&item); err != nil {
			return fmt.Errorf("decoding item %q from %s: %w", k, collection, err)
		}

		if item.Status != constants.SplitStatusActive {
			return nil
		}

		var dto T
		if err := json.Unmarshal([]byte(item.JSON), &dto); err != nil {
			return fmt.Errorf("decoding definition of %q from %s: %w", item.Name, collection, err)
		}

		canonical, err := json.Marshal(dto)
		if err != nil {
			return fmt.Errorf("encoding definition of %q from %s: %w", item.Name, collection, err)
		}

		entries = append(entries, contentEntry{
			name:         item.Name,
			changeNumber: item.ChangeNumber,
			canonical:    canonical,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// segmentContent is the logical content of a segment: its name and the sorted names of its active keys
type segmentContent struct {
	name string
	keys []string
}

// segmentsContent returns every segment with its non-removed keys. Segments whose keys were all removed are kept.
func segmentsContent(tx *bolt.Tx) ([]segmentContent, error) {
	bucket := tx.Bucket([]byte(segmentChangesCollectionName))
	if bucket == nil {
		return nil, nil
	}

	var segments []segmentContent
	err := bucket.ForEach(func(k, v []byte) error {
		var item SegmentChangesItem
		if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&item); err != nil {
			return fmt.Errorf("decoding segment %q: %w", k, err)
		}

		keys := make([]string, 0, len(item.Keys))
		for name, key := range item.Keys {
			if !key.Removed {
				keys = append(keys, name)
			}
		}
		sort.Strings(keys)

		segments = append(segments, segmentContent{name: item.Name, keys: keys})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return segments, nil
}

// computeContentID returns a sha256 hex digest over the logical content of the db (ACTIVE flags,
// ACTIVE rule-based segments, segment keys) plus the flag spec version and flag sets.
// It must be called within the same transaction used to export the raw db bytes.
func computeContentID(tx *bolt.Tx, flagSpecVersion string, flagSets []string) (string, error) {
	flags, err := activeChangesEntries[dtos.SplitDTO](tx, splitChangesCollectionName)
	if err != nil {
		return "", fmt.Errorf("computing content id, flags: %w", err)
	}

	ruleBased, err := activeChangesEntries[dtos.RuleBasedSegmentDTO](tx, ruleBasedSegmentsChangesCollectionName)
	if err != nil {
		return "", fmt.Errorf("computing content id, rule-based segments: %w", err)
	}

	segments, err := segmentsContent(tx)
	if err != nil {
		return "", fmt.Errorf("computing content id, segments: %w", err)
	}

	sets := make([]string, len(flagSets))
	copy(sets, flagSets)
	sort.Strings(sets)
	sets = slices.Compact(sets)

	h := sha256.New()
	w := &contentWriter{h: h}

	w.writeString(contentIDVersion)
	w.writeString(contentIDSectionSpecVersion)
	w.writeString(flagSpecVersion)

	w.writeString(contentIDSectionFlagSets)
	w.writeCount(len(sets))
	for _, s := range sets {
		w.writeString(s)
	}

	w.writeString(contentIDSectionFlags)
	w.writeCount(len(flags))
	for _, e := range flags {
		w.writeString(e.name)
		w.writeInt64(e.changeNumber)
		w.writeBytes(e.canonical)
	}

	w.writeString(contentIDSectionRBSegments)
	w.writeCount(len(ruleBased))
	for _, e := range ruleBased {
		w.writeString(e.name)
		w.writeInt64(e.changeNumber)
		w.writeBytes(e.canonical)
	}

	w.writeString(contentIDSectionSegments)
	w.writeCount(len(segments))
	for _, s := range segments {
		w.writeString(s.name)
		w.writeCount(len(s.keys))
		for _, k := range s.keys {
			w.writeString(k)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
