package flagsets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlagSetsMatcher(t *testing.T) {

	m := NewMatcher(false, []string{"s1", "s2", "s3"})
	assert.Equal(t, []string{"s1", "s2", "s3"}, m.Sanitize([]string{"s1", "s2", "s3"}))
	assert.Equal(t, []string{"s1", "s2"}, m.Sanitize([]string{"s1", "s2"}))
	assert.Equal(t, []string{"s4"}, m.Sanitize([]string{"s4"}))

	m = NewMatcher(true, []string{"s1", "s2", "s3"})
	assert.Equal(t, []string{"s1", "s2", "s3"}, m.Sanitize([]string{"s1", "s2", "s3"}))
	assert.Equal(t, []string{"s1", "s2"}, m.Sanitize([]string{"s1", "s2"}))
	assert.Equal(t, []string{"s1", "s2"}, m.Sanitize([]string{"s1", "s2", "s7"}))
	assert.Equal(t, []string{}, m.Sanitize([]string{"s4"}))
}

func TestFlagSetsMatcherSanitizeStrict(t *testing.T) {
	m := NewMatcher(true, []string{"s1", "s2"})

	// consecutive invalid sets must all be removed
	assert.Equal(t, []string{}, m.Sanitize([]string{"bad1", "bad2"}))
	assert.Equal(t, []string{"s1"}, m.Sanitize([]string{"bad1", "s1", "bad2", "bad3"}))
	assert.Equal(t, []string{"s1", "s2"}, m.Sanitize([]string{"bad1", "bad2", "bad3", "s2", "s1"}))
	assert.Equal(t, []string{"s1"}, m.Sanitize([]string{"s1", "bad1", "bad2"}))

	// invalid + duplicates
	assert.Equal(t, []string{"s1", "s2"}, m.Sanitize([]string{"s2", "bad", "s1", "bad", "s2", "s1"}))
}

func TestFlagSetsMatcherSanitizeDuplicates(t *testing.T) {
	m := NewMatcher(false, nil)
	assert.Equal(t, []string{"a"}, m.Sanitize([]string{"a", "a", "a"}))
	assert.Equal(t, []string{"a", "b"}, m.Sanitize([]string{"a", "b", "b", "a"}))
	assert.Equal(t, []string{"a", "b", "c"}, m.Sanitize([]string{"c", "a", "b", "a", "c", "b", "a"}))
}

func TestFlagSetsMatcherSanitizeDoesNotMutateInput(t *testing.T) {
	m := NewMatcher(false, nil)

	raw := []string{"c", "b", "a"}
	assert.Equal(t, []string{"a", "b", "c"}, m.Sanitize(raw))
	assert.Equal(t, []string{"c", "b", "a"}, raw)

	dups := []string{"a", "a", "b"}
	assert.Equal(t, []string{"a", "b"}, m.Sanitize(dups))
	assert.Equal(t, []string{"a", "a", "b"}, dups)

	strict := NewMatcher(true, []string{"s1"})
	invalid := []string{"x", "s1", "y"}
	assert.Equal(t, []string{"s1"}, strict.Sanitize(invalid))
	assert.Equal(t, []string{"x", "s1", "y"}, invalid)
}

func TestFlagSetsMatcherSanitizeEmpty(t *testing.T) {
	m := NewMatcher(false, nil)
	assert.Nil(t, m.Sanitize(nil))
	assert.Equal(t, []string{}, m.Sanitize([]string{}))
}
