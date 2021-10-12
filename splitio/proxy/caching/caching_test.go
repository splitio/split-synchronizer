package caching

import (
	"testing"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/testhelpers"
)

func TestSegment(t *testing.T) {

	if MakeSurrogateForSegmentChanges("segment1") != segmentPrefix+"segment1" {
		t.Error("wrong segment changes surrogate.")
	}
}

func TestMySegments(t *testing.T) {
	testhelpers.AssertStringSliceEquals(
		t,
		MakeSurrogateForMySegments([]dtos.MySegmentDTO{{Name: "segment1"}, {Name: "segment2"}}),
		[]string{segmentPrefix + "segment1", segmentPrefix + "segment2"},
		"wrong my segments surrogate keys",
	)
}
