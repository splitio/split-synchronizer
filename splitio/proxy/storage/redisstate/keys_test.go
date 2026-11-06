package redisstate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func sha16(apikey string) string {
	sum := sha256.Sum256([]byte(apikey))
	return hex.EncodeToString(sum[:8])
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
		assert.Equal(t, []string{base + ".meta", base + ".seg.a.b", base + ".segrm.a.b", base + ".segat.a.b", base + ".segments"}, k.SegmentKeys("a.b"))
		assert.Equal(t, "flags.till", k.FlagCursorField())
		assert.Equal(t, "rbs.till", k.RuleBasedSegmentCursorField())
		assert.Equal(t, "seg.a.b.till", k.SegmentCursorField("a.b"))
		assert.Equal(t, base+".*", k.Pattern())
	})

	t.Run("layout without prefix", func(t *testing.T) {
		k, err := NewKeys("", "secret-api-key", "")
		assert.Nil(t, err)
		assert.Equal(t, "{SPLITIO_PROXY."+env+"}.meta", k.Meta())
		assert.Equal(t, "{SPLITIO_PROXY."+env+"}.*", k.Pattern())
	})

	t.Run("stable and does not contain the api key", func(t *testing.T) {
		k1, err1 := NewKeys("p", "secret-api-key", "")
		k2, err2 := NewKeys("p", "secret-api-key", "")
		assert.Nil(t, err1)
		assert.Nil(t, err2)
		assert.Equal(t, k1, k2)
		all := append(k1.SegmentKeys("x"), k1.FlagKeys()...)
		all = append(all, k1.Pattern(), k1.Env())
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
		all = append(all, k.Meta(), k.Pattern())
		for _, key := range all {
			assert.False(t, strings.HasPrefix(key, "SPLITIO."), key)
			if prefix != "" {
				assert.False(t, strings.HasPrefix(key, prefix+".SPLITIO."), key)
			}
		}
	}
}
