// Package redisstate writes the state of the connected proxy to Redis.
package redisstate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	keyRoot   = "SPLITIO_PROXY"
	globChars = `*?[]\`
	// hashTagChars would corrupt the cluster hash tag {SPLITIO_PROXY.<env>} if they appeared in the prefix
	hashTagChars = "{}"
	// reservedKeyPrefix is the key namespace the synchronizer uses; the proxy must never write under it
	reservedKeyPrefix = "SPLITIO."
)

const (
	// FingerprintField is the meta hash field holding the environment fingerprint. The Lua scripts keep the same literal.
	FingerprintField = "fingerprint"
	// UpdatedAtField is the meta hash field holding the time in ms of the last applied write. The Lua scripts keep the same literal.
	UpdatedAtField = "updatedAtMs"
)

var namespaceRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var (
	// ErrInvalidPrefix is returned when the Redis prefix contains glob characters
	ErrInvalidPrefix = errors.New("redis prefix must not contain any of *?[]\\{}")
	// ErrReservedPrefix is returned when the prefix would place proxy keys under the synchronizer's SPLITIO. namespace
	ErrReservedPrefix = errors.New("redis prefix must not start with SPLITIO. (reserved for the synchronizer)")
	// ErrInvalidNamespace is returned when the namespace does not match ^[A-Za-z0-9_-]{1,64}$
	ErrInvalidNamespace = errors.New("redis state namespace must match ^[A-Za-z0-9_-]{1,64}$")
	// ErrMissingIdentity is returned when neither a namespace nor an api key is available to identify the environment
	ErrMissingIdentity = errors.New("an api key or a namespace is required to name the redis state")
)

// Keys builds every Redis key name used by the proxy state of one environment
type Keys struct {
	env    string
	prefix string
	base   string
}

// NewKeys builds the key set for an environment. The environment id is the namespace when it is not
// empty, otherwise the first 16 hex characters of the SHA-256 of the api key.
func NewKeys(prefix, apikey, namespace string) (Keys, error) {
	if strings.ContainsAny(prefix, globChars+hashTagChars) {
		return Keys{}, ErrInvalidPrefix
	}

	var env string
	switch {
	case namespace != "":
		if !namespaceRegex.MatchString(namespace) {
			return Keys{}, ErrInvalidNamespace
		}
		env = namespace
	case apikey != "":
		sum := sha256.Sum256([]byte(apikey))
		env = hex.EncodeToString(sum[:8])
	default:
		return Keys{}, ErrMissingIdentity
	}

	base := "{" + keyRoot + "." + env + "}"
	if prefix != "" {
		base = prefix + "." + base
		if strings.HasPrefix(base, reservedKeyPrefix) {
			return Keys{}, ErrReservedPrefix
		}
	}
	return Keys{env: env, prefix: prefix, base: base}, nil
}

// Env returns the environment id used in the key names
func (k Keys) Env() string {
	return k.env
}

// Meta returns the prefixed name of the meta hash (fingerprint and cursors)
func (k Keys) Meta() string {
	return k.base + ".meta"
}

// MetaUnprefixed returns the meta hash name without the Redis prefix, for clients that add the prefix themselves
func (k Keys) MetaUnprefixed() string {
	return "{" + keyRoot + "." + k.env + "}.meta"
}

// Unprefixed returns the key set without the Redis prefix, for clients that add the prefix themselves
func (k Keys) Unprefixed() Keys {
	return Keys{env: k.env, base: "{" + keyRoot + "." + k.env + "}"}
}

// FlagData returns the hash holding the JSON of every feature flag, by name
func (k Keys) FlagData() string {
	return k.base + ".flags"
}

// FlagIndex returns the sorted set of feature flag names, scored by change number
func (k Keys) FlagIndex() string {
	return k.base + ".flags.index"
}

// FlagRemoved returns the sorted set of removed feature flag names, scored by the removal time in ms
func (k Keys) FlagRemoved() string {
	return k.base + ".flags.removed"
}

// RuleBasedSegmentData returns the hash holding the JSON of every rule-based segment, by name
func (k Keys) RuleBasedSegmentData() string {
	return k.base + ".rbs"
}

// RuleBasedSegmentIndex returns the sorted set of rule-based segment names, scored by change number
func (k Keys) RuleBasedSegmentIndex() string {
	return k.base + ".rbs.index"
}

// RuleBasedSegmentRemoved returns the sorted set of removed rule-based segment names, scored by the removal time in ms
func (k Keys) RuleBasedSegmentRemoved() string {
	return k.base + ".rbs.removed"
}

// SegmentActive returns the sorted set of keys present in the named segment, scored by change number
func (k Keys) SegmentActive(name string) string {
	return k.base + ".seg." + name
}

// SegmentTombstones returns the sorted set of keys removed from the named segment, scored by change number
func (k Keys) SegmentTombstones(name string) string {
	return k.base + ".segrm." + name
}

// SegmentRemovalClock returns the sorted set of tombstone times in ms of the named segment, scored by removal time
func (k Keys) SegmentRemovalClock(name string) string {
	return k.base + ".segat." + name
}

// FlagKeys returns the keys the entity script touches for feature flags: meta, data, index, removed
func (k Keys) FlagKeys() []string {
	return []string{k.Meta(), k.FlagData(), k.FlagIndex(), k.FlagRemoved()}
}

// RuleBasedSegmentKeys returns the keys the entity script touches for rule-based segments: meta, data, index, removed
func (k Keys) RuleBasedSegmentKeys() []string {
	return []string{k.Meta(), k.RuleBasedSegmentData(), k.RuleBasedSegmentIndex(), k.RuleBasedSegmentRemoved()}
}

// SegmentKeys returns the keys the segment script touches: meta, active, tombstones, removal clock
func (k Keys) SegmentKeys(name string) []string {
	return []string{k.Meta(), k.SegmentActive(name), k.SegmentTombstones(name), k.SegmentRemovalClock(name)}
}

// FlagCursorField returns the meta hash field holding the feature flag cursor
func (k Keys) FlagCursorField() string {
	return "flags.till"
}

// RuleBasedSegmentCursorField returns the meta hash field holding the rule-based segment cursor
func (k Keys) RuleBasedSegmentCursorField() string {
	return "rbs.till"
}

// SegmentCursorField returns the meta hash field holding the cursor of the named segment
func (k Keys) SegmentCursorField(name string) string {
	return fmt.Sprintf("seg.%s.till", name)
}

// PurgeLast is the key of the count the last PurgeScript call removed. It shares the hash tag of the other keys.
func (k Keys) PurgeLast() string {
	return k.base + ".purgelast"
}
