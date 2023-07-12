package caching

import (
	"testing"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-toolkit/v5/testhelpers"
)

func TestSegment(t *testing.T) {

	if MakeSurrogateForSegmentChanges("segment1") != segmentPrefix+"segment1" {
		t.Error("wrong segment changes surrogate.")
	}
}

func TestMySegmentKeyGeneration(t *testing.T) {
	entries := MakeMySegmentsEntries("k1")
	if entries[0] != "/api/mySegments/k1" {
		t.Error("invalid mySegments cache entry")
	}
	if entries[1] != "gzip::/api/mySegments/k1" {
		t.Error("invalid mySegments cache entry")
	}
}

func TestMySegments(t *testing.T) {
	testhelpers.AssertStringSliceEquals(
		t,
		MakeSurrogateForMySegments([]dtos.MySegmentDTO{{Name: "segment1"}, {Name: "segment2"}}),
		[]string{},
		"wrong my segments surrogate keys",
	)
}
