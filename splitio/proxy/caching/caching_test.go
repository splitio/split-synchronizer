package caching

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/stretchr/testify/assert"
)

func TestCacheKeysDoNotOverlap(t *testing.T) {

	url1, _ := url.Parse("http://proxy.split.io/api/spitChanges?since=-1")
	c1 := &gin.Context{Request: &http.Request{URL: url1}}

	url2, _ := url.Parse("http://proxy.split.io/api/spitChanges?s=1.1&since=-1")
	c2 := &gin.Context{Request: &http.Request{URL: url2}}

	assert.NotEqual(t, keyFactoryFN(c1), keyFactoryFN(c2))
}

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
