package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTillCapsApply(t *testing.T) {
	var c tillCaps
	assert.Equal(t, int64(9), c.apply("a", 9, 5), "no ceiling leaves the till")

	c.set("a", 7)
	assert.Equal(t, int64(7), c.apply("a", 9, 5), "till above the ceiling is capped")
	assert.Equal(t, int64(6), c.apply("a", 6, 5), "till below the ceiling is kept")
	assert.Equal(t, int64(8), c.apply("a", 9, 8), "never below since")
	assert.Equal(t, int64(9), c.apply("b", 9, 5), "other keys are not capped")

	c.lift()
	assert.Equal(t, int64(9), c.apply("a", 9, 5), "lifted")
}
