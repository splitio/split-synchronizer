package redisstate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sha16(apikey string) string {
	sum := sha256.Sum256([]byte(apikey))
	return hex.EncodeToString(sum[:8])
}

func TestKeyAccessorsAreTheSliceLayout(t *testing.T) {
	k, err := NewKeys("pre", "secret-api-key", "")
	assert.Nil(t, err)
	base := "pre.{SPLITIO_PROXY." + sha16("secret-api-key") + "}"

	assert.Equal(t, base+".flags", k.FlagData())
	assert.Equal(t, base+".flags.index", k.FlagIndex())
	assert.Equal(t, base+".flags.removed", k.FlagRemoved())
	assert.Equal(t, []string{k.Meta(), k.FlagData(), k.FlagIndex(), k.FlagRemoved()}, k.FlagKeys())

	assert.Equal(t, base+".rbs", k.RuleBasedSegmentData())
	assert.Equal(t, base+".rbs.index", k.RuleBasedSegmentIndex())
	assert.Equal(t, base+".rbs.removed", k.RuleBasedSegmentRemoved())
	assert.Equal(t, []string{k.Meta(), k.RuleBasedSegmentData(), k.RuleBasedSegmentIndex(), k.RuleBasedSegmentRemoved()}, k.RuleBasedSegmentKeys())

	assert.Equal(t, base+".seg.s", k.SegmentActive("s"))
	assert.Equal(t, base+".segrm.s", k.SegmentTombstones("s"))
	assert.Equal(t, base+".segat.s", k.SegmentRemovalClock("s"))
	assert.Equal(t, []string{k.Meta(), k.SegmentActive("s"), k.SegmentTombstones("s"), k.SegmentRemovalClock("s")}, k.SegmentKeys("s"))

	assert.Equal(t, "fingerprint", FingerprintField)
	assert.Equal(t, "updatedAtMs", UpdatedAtField)
}

func TestNewKeys(t *testing.T) {
	env := sha16("secret-api-key")

	t.Run("layout with prefix", func(t *testing.T) {
		k, err := NewKeys("pre", "secret-api-key", "")
		assert.Nil(t, err)
		base := "pre.{SPLITIO_PROXY." + env + "}"
		assert.Equal(t, env, k.Env())
		assert.Equal(t, base+".meta", k.Meta())
		assert.Equal(t, "{SPLITIO_PROXY."+env+"}.meta", k.MetaUnprefixed())
		assert.Equal(t, []string{base + ".meta", base + ".flags", base + ".flags.index", base + ".flags.removed"}, k.FlagKeys())
		assert.Equal(t, []string{base + ".meta", base + ".rbs", base + ".rbs.index", base + ".rbs.removed"}, k.RuleBasedSegmentKeys())
		assert.Equal(t, []string{base + ".meta", base + ".seg.a.b", base + ".segrm.a.b", base + ".segat.a.b"}, k.SegmentKeys("a.b"))
		assert.Equal(t, "flags.till", k.FlagCursorField())
		assert.Equal(t, "rbs.till", k.RuleBasedSegmentCursorField())
		assert.Equal(t, "seg.a.b.till", k.SegmentCursorField("a.b"))
	})

	t.Run("layout without prefix", func(t *testing.T) {
		k, err := NewKeys("", "secret-api-key", "")
		assert.Nil(t, err)
		assert.Equal(t, "{SPLITIO_PROXY."+env+"}.meta", k.Meta())
	})

	t.Run("stable and does not contain the api key", func(t *testing.T) {
		k1, err1 := NewKeys("p", "secret-api-key", "")
		k2, err2 := NewKeys("p", "secret-api-key", "")
		assert.Nil(t, err1)
		assert.Nil(t, err2)
		assert.Equal(t, k1, k2)
		all := append(k1.SegmentKeys("x"), k1.FlagKeys()...)
		all = append(all, k1.Env())
		for _, key := range all {
			assert.False(t, strings.Contains(key, "secret-api-key"))
		}
	})

	t.Run("rejected inputs", func(t *testing.T) {
		for _, c := range []string{"*", "?", "[", "]", `\`} {
			_, err := NewKeys("a"+c+"b", "key", "")
			assert.NotNil(t, err, "prefix %q", c)
			assert.False(t, strings.Contains(err.Error(), "key\""))
		}
		_, err := NewKeys("p", "", "")
		assert.NotNil(t, err)
	})

	t.Run("reserved synchronizer namespace", func(t *testing.T) {
		for _, p := range []string{"SPLITIO", "SPLITIO.app"} {
			_, err := NewKeys(p, "key", "")
			assert.ErrorIs(t, err, ErrReservedPrefix, "prefix %q", p)
		}
		_, err := NewKeys("SPLITIOX", "key", "")
		assert.Nil(t, err, "a prefix that only starts with the letters SPLITIO is allowed")
	})
}

func TestNewKeysEnvIsSha256Prefix(t *testing.T) {
	k1, err := NewKeys("p", "api-key-1", "")
	assert.Nil(t, err)
	k2, err := NewKeys("p", "api-key-2", "")
	assert.Nil(t, err)
	assert.Equal(t, sha16("api-key-1"), k1.Env())
	assert.Equal(t, sha16("api-key-2"), k2.Env())
	assert.Len(t, k1.Env(), 16)
	assert.NotEqual(t, k1.Env(), k2.Env())
	assert.Equal(t, strings.ToLower(k1.Env()), k1.Env())
}

func TestNewKeysNamespaceOverride(t *testing.T) {
	k1, err := NewKeys("p", "api-key-1", "my-env_1")
	assert.Nil(t, err)
	k2, err := NewKeys("p", "totally-different", "my-env_1")
	assert.Nil(t, err)
	k3, err := NewKeys("p", "", "my-env_1")
	assert.Nil(t, err)
	assert.Equal(t, "my-env_1", k1.Env())
	assert.Equal(t, k1, k2)
	assert.Equal(t, k1, k3)
	assert.Equal(t, "p.{SPLITIO_PROXY.my-env_1}.meta", k1.Meta())
}

func TestNewKeysRejectsBadNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		ok        bool
	}{
		{"empty is allowed", "", true},
		{"dot", "a.b", false},
		{"braces", "a{b}", false},
		{"star", "a*", false},
		{"space", "a b", false},
		{"64 chars", strings.Repeat("a", 64), true},
		{"65 chars", strings.Repeat("a", 65), false},
		{"non ascii", "é", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKeys("p", "api-key", tc.namespace)
			if tc.ok {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestNewKeysDoesNotMatchSynchronizerNamespace(t *testing.T) {
	for _, prefix := range []string{"", "pre"} {
		k, err := NewKeys(prefix, "api-key", "")
		assert.Nil(t, err)
		all := append(k.SegmentKeys("x"), k.FlagKeys()...)
		all = append(all, k.RuleBasedSegmentKeys()...)
		all = append(all, k.Meta())
		for _, key := range all {
			assert.False(t, strings.HasPrefix(key, "SPLITIO."), key)
			if prefix != "" {
				assert.False(t, strings.HasPrefix(key, prefix+".SPLITIO."), key)
			}
		}
	}
}

func TestKeysPrefixRejectsBraces(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ok     bool
	}{
		{"normal prefix", "pre", true},
		{"open brace", "a{b", false},
		{"close brace", "a}b", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKeys(tc.prefix, "secret-api-key", "")
			if tc.ok {
				assert.Nil(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidPrefix)
			}
		})
	}
}

func TestNewKeysRejectsBraces(t *testing.T) {
	for _, ns := range []string{"a{b", "a}b", "{x}", "x}{"} {
		_, err := NewKeys("pre", "secret-api-key", ns)
		assert.ErrorIs(t, err, ErrInvalidNamespace, ns)
	}
}

func TestPurgeLastSharesHashTag(t *testing.T) {
	k, err := NewKeys("app", "test-api-key", "")
	require.NoError(t, err)
	assert.Equal(t, "{SPLITIO_PROXY."+k.Env()+"}.purgelast", k.Unprefixed().PurgeLast())
	assert.Equal(t, "app."+k.Unprefixed().PurgeLast(), k.PurgeLast(), "the prefixed name adds the prefix only")
}
