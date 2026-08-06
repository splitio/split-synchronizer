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
