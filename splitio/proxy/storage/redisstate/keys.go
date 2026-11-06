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
	// reservedKeyPrefix is the key namespace the synchronizer uses; the proxy must never write under it
	reservedKeyPrefix = "SPLITIO."
)

var namespaceRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var (
	// ErrInvalidPrefix is returned when the Redis prefix contains glob characters
	ErrInvalidPrefix = errors.New("redis prefix must not contain any of *?[]\\")
	// ErrReservedPrefix is returned when the prefix would place proxy keys under the synchronizer's SPLITIO. namespace
	ErrReservedPrefix = errors.New("redis prefix must not start with SPLITIO. (reserved for the synchronizer)")
	// ErrInvalidNamespace is returned when the namespace does not match ^[A-Za-z0-9_-]{1,64}$
	ErrInvalidNamespace = errors.New("redis state namespace must match ^[A-Za-z0-9_-]{1,64}$")
	// ErrMissingIdentity is returned when neither a namespace nor an api key is available to identify the environment
	ErrMissingIdentity = errors.New("an api key or a namespace is required to name the redis state")
)

// Keys builds every Redis key name used by the proxy state of one environment
type Keys struct {
	env  string
	base string
}

// NewKeys builds the key set for an environment. The environment id is the namespace when it is not
// empty, otherwise the first 16 hex characters of the SHA-256 of the api key.
func NewKeys(prefix, apikey, namespace string) (Keys, error) {
	if strings.ContainsAny(prefix, globChars) {
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
	return Keys{env: env, base: base}, nil
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

// FlagKeys returns the keys the entity script touches for feature flags: meta, data, index, removed
func (k Keys) FlagKeys() []string {
	return []string{k.Meta(), k.base + ".flags", k.base + ".flags.index", k.base + ".flags.removed"}
}

// RuleBasedSegmentKeys returns the keys the entity script touches for rule-based segments: meta, data, index, removed
func (k Keys) RuleBasedSegmentKeys() []string {
	return []string{k.Meta(), k.base + ".rbs", k.base + ".rbs.index", k.base + ".rbs.removed"}
}

// SegmentKeys returns the keys the segment script touches: meta, active, tombstones, removal clock, segment names
func (k Keys) SegmentKeys(name string) []string {
	return []string{k.Meta(), k.base + ".seg." + name, k.base + ".segrm." + name, k.base + ".segat." + name, k.base + ".segments"}
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

// Pattern returns the glob matching every key of this environment, used only by Reset
func (k Keys) Pattern() string {
	return k.base + ".*"
}
