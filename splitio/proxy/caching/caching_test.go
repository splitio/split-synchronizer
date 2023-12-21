package caching

import (
	"testing"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/stretchr/testify/assert"
)

func TestSegmentSurrogates(t *testing.T) {
	assert.Equal(t, segmentPrefix+"segment1", MakeSurrogateForSegmentChanges("segment1"))
	assert.NotEqual(t, MakeSurrogateForSegmentChanges("segment1"), MakeSurrogateForSegmentChanges("segment2"))
}

func TestMySegmentKeyGeneration(t *testing.T) {
	entries := MakeMySegmentsEntries("k1")
	assert.Equal(t, "/api/mySegments/k1", entries[0])
	assert.Equal(t, "gzip::/api/mySegments/k1", entries[1])
}

func TestMySegmentsSurrogates(t *testing.T) {
	assert.Equal(t, []string(nil), MakeSurrogateForMySegments([]dtos.MySegmentDTO{{Name: "segment1"}, {Name: "segment2"}}))
}
