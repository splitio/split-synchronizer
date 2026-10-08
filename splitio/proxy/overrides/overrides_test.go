package overrides

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/stretchr/testify/assert"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	assert.Nil(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadValid(t *testing.T) {
	t.Run("empty file has no entries", func(t *testing.T) {
		o, err := Load(writeFile(t, ""))
		assert.Nil(t, err)
		assert.Empty(t, o.Entries)
	})

	t.Run("empty map has no entries", func(t *testing.T) {
		o, err := Load(writeFile(t, "{}"))
		assert.Nil(t, err)
		assert.Empty(t, o.Entries)
	})

	t.Run("comments only has no entries", func(t *testing.T) {
		o, err := Load(writeFile(t, "# INC-1: nothing yet\n"))
		assert.Nil(t, err)
		assert.Empty(t, o.Entries)
	})

	t.Run("short form", func(t *testing.T) {
		o, err := Load(writeFile(t, "# INC-4812\nnew_checkout: \"off\"\n"))
		assert.Nil(t, err)
		assert.Equal(t, Entry{Flag: "new_checkout", Treatment: "off"}, o.Entries["new_checkout"])
	})

	t.Run("unquoted string treatment", func(t *testing.T) {
		o, err := Load(writeFile(t, "new_checkout: control\n"))
		assert.Nil(t, err)
		assert.Equal(t, "control", o.Entries["new_checkout"].Treatment)
	})

	t.Run("unquoted on and off are strings (YAML 1.2)", func(t *testing.T) {
		o, err := Load(writeFile(t, "a: off\nb:\n  treatment: on\n"))
		assert.Nil(t, err)
		assert.Equal(t, "off", o.Entries["a"].Treatment)
		assert.Equal(t, "on", o.Entries["b"].Treatment)
	})

	t.Run("long form without config keeps existing config", func(t *testing.T) {
		o, err := Load(writeFile(t, "search_rerank:\n  treatment: \"v2\"\n"))
		assert.Nil(t, err)
		assert.Equal(t, Entry{Flag: "search_rerank", Treatment: "v2"}, o.Entries["search_rerank"])
		assert.Nil(t, o.Entries["search_rerank"].Config)
	})

	t.Run("long form with config", func(t *testing.T) {
		o, err := Load(writeFile(t, "search_rerank:\n  treatment: \"v2\"\n  config: '{\"model\":\"small\"}'\n"))
		assert.Nil(t, err)
		e := o.Entries["search_rerank"]
		assert.Equal(t, "v2", e.Treatment)
		if assert.NotNil(t, e.Config) {
			assert.Equal(t, `{"model":"small"}`, *e.Config)
		}
	})

	t.Run("file order is preserved", func(t *testing.T) {
		o, err := Load(writeFile(t, "zeta: a\nalpha: b\nmid: c\n"))
		assert.Nil(t, err)
		assert.Equal(t, []string{"zeta", "alpha", "mid"}, o.Flags())
	})

	t.Run("hash is the sha256 of the raw bytes", func(t *testing.T) {
		content := "new_checkout: \"off\"\n# note\n"
		o, err := Load(writeFile(t, content))
		assert.Nil(t, err)
		sum := sha256.Sum256([]byte(content))
		assert.Equal(t, hex.EncodeToString(sum[:]), o.SHA256)
	})
}

func TestLoadInvalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
		flag    string // expected in the error; empty when not tied to one flag
		field   string // expected in the error; empty when not tied to one field
		extra   string // additional substring
	}{
		{name: "malformed yaml", content: "new_checkout: [unterminated\n"},
		{name: "duplicate key", content: "a: x\nb: y\na: z\n", flag: "a", extra: "duplicate"},
		{name: "root is a list", content: "- a\n- b\n", extra: "map"},
		{name: "root is a scalar", content: "just-a-string\n", extra: "map"},
		{name: "unknown field", content: "a:\n  treatment: x\n  foo: y\n", flag: "a", field: "foo"},
		{name: "keys field gets a specific message", content: "a:\n  treatment: x\n  keys: [u1]\n", flag: "a", field: "keys", extra: "per-key targeting is not supported"},
		{name: "long form without treatment", content: "a:\n  config: '{}'\n", flag: "a", field: "treatment"},
		{name: "boolean treatment short form", content: "a: false\n", flag: "a", field: "treatment", extra: "quote"},
		{name: "boolean treatment long form", content: "a:\n  treatment: true\n", flag: "a", field: "treatment", extra: "quote"},
		{name: "numeric treatment", content: "a: 5\n", flag: "a", field: "treatment", extra: "quote"},
		{name: "config is not a string", content: "a:\n  treatment: x\n  config:\n    model: small\n", flag: "a", field: "config"},
		{name: "config is not valid json", content: "a:\n  treatment: x\n  config: '{nope'\n", flag: "a", field: "config", extra: "JSON"},
		{name: "entry is a list", content: "a:\n  - x\n", flag: "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, tc.content)
			o, err := Load(path)
			assert.Nil(t, o)
			if !assert.NotNil(t, err) {
				return
			}
			assert.Contains(t, err.Error(), path)
			if tc.flag != "" {
				assert.Contains(t, err.Error(), tc.flag)
			}
			if tc.field != "" {
				assert.Contains(t, err.Error(), tc.field)
			}
			if tc.extra != "" {
				assert.Contains(t, err.Error(), tc.extra)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nope.yaml")
		o, err := Load(path)
		assert.Nil(t, o)
		if assert.NotNil(t, err) {
			assert.Contains(t, err.Error(), path)
		}
	})
}

func flagWith(name, def string, treatments ...string) dtos.SplitDTO {
	parts := make([]dtos.PartitionDTO, 0, len(treatments))
	for _, tr := range treatments {
		parts = append(parts, dtos.PartitionDTO{Treatment: tr, Size: 100 / len(treatments)})
	}
	return dtos.SplitDTO{
		Name:             name,
		DefaultTreatment: def,
		Conditions:       []dtos.ConditionDTO{{ConditionType: "ROLLOUT", Partitions: parts}},
	}
}

func mustValidate(t *testing.T, o *Overrides, flags []dtos.SplitDTO) []string {
	t.Helper()
	skipped, err := Validate(o, flags)
	assert.Nil(t, err)
	return skipped
}

func TestValidate(t *testing.T) {
	flags := []dtos.SplitDTO{
		flagWith("new_checkout", "off", "on", "off"),
		flagWith("search_rerank", "v1", "v1", "v2"),
	}

	t.Run("treatments from rules and default are accepted", func(t *testing.T) {
		path := writeFile(t, "new_checkout: \"on\"\nsearch_rerank: v2\n")
		o, err := Load(path)
		assert.Nil(t, err)
		mustValidate(t, o, flags)
		assert.Len(t, o.Entries, 2)
	})

	t.Run("default treatment absent from the rules is accepted", func(t *testing.T) {
		path := writeFile(t, "f: fallback\n")
		o, err := Load(path)
		assert.Nil(t, err)
		mustValidate(t, o, []dtos.SplitDTO{flagWith("f", "fallback", "a", "b")})
	})

	t.Run("treatment the flag does not have fails", func(t *testing.T) {
		path := writeFile(t, "new_checkout: maybe\n")
		o, err := Load(path)
		assert.Nil(t, err)
		_, err = Validate(o, flags)
		if assert.NotNil(t, err) {
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), "new_checkout")
			assert.Contains(t, err.Error(), "treatment")
			assert.Contains(t, err.Error(), "maybe")
		}
	})

	t.Run("flag missing from the snapshot is dropped, not an error", func(t *testing.T) {
		path := writeFile(t, "old_flag: x\nnew_checkout: \"on\"\n")
		o, err := Load(path)
		assert.Nil(t, err)
		assert.Equal(t, []string{"old_flag"}, mustValidate(t, o, flags))
		assert.Len(t, o.Entries, 1)
		assert.Contains(t, o.Entries, "new_checkout")
		assert.Equal(t, []string{"new_checkout"}, o.Flags())
	})

	t.Run("empty overrides validate", func(t *testing.T) {
		o, err := Load(writeFile(t, ""))
		assert.Nil(t, err)
		mustValidate(t, o, flags)
	})
}
