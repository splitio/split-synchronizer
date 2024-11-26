package caching

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v6/dtos"
)

const (
	// SurrogateContextKey is the gin context key used to store surrogates generated on each response
	SurrogateContextKey = "surrogates"

	// StickyContextKey should be set to (boolean) true whenever we want an entry to be kept in cache when making room
	// for new entries
	StickyContextKey = gincache.StickyEntry

	// SplitSurrogate key (we only need one, since all splitChanges should be expired when an update is processed)
	SplitSurrogate = "sp"

	// LargeSegmentSurrogate key (we only need one, since all memberships should be expired when an update is processed)
	LargeSegmentSurrogate = "ls"

	// AuthSurrogate key (having push disabled, it's safe to cache this and return it on all requests)
	AuthSurrogate = "au"

	segmentPrefix = "se::"
)

const cacheSize = 1000000

// MakeSurrogateForSegmentChanges creates a surrogate key for the segment being queried
func MakeSurrogateForSegmentChanges(segmentName string) string {
	return segmentPrefix + segmentName
}

// MakeSurrogateForMySegments creates a list surrogate keys for all the segments involved
func MakeSurrogateForMySegments(mysegments []dtos.MySegmentDTO) []string {
	// Since we are now evicting individually for every updated key, we don't need surrogates for mySegments
	return nil
}

// MakeMySegmentsEntry create a cache entry key for mysegments
func MakeMySegmentsEntries(key string) []string {
	return []string{
		"/api/mySegments/" + key,
		"gzip::/api/mySegments/" + key,
	}
}

// MakeProxyCache creates and configures a split-proxy-ready cache
func MakeProxyCache() *gincache.Middleware {
	return gincache.New(&gincache.Options{
		SuccessfulOnly: true, // we're not interested in caching non-200 responses
		Size:           cacheSize,
		KeyFactory:     keyFactoryFN,
		// we make each request handler responsible for generating the surrogates.
		// this way we can use segment names as surrogates for mysegments & segment changes
		// with a lot less work
		SurrogateFactory: func(ctx *gin.Context) []string { return ctx.GetStringSlice(SurrogateContextKey) },
	})
}

func keyFactoryFN(ctx *gin.Context) string {
	var encodingPrefix string
	if strings.Contains(ctx.Request.Header.Get("Accept-Encoding"), "gzip") {
		encodingPrefix = "gzip::"
	}

	if strings.HasPrefix(ctx.Request.URL.Path, "/api/auth") || strings.HasPrefix(ctx.Request.URL.Path, "/api/v2/auth") {
		// For auth requests, since we don't support streaming yet, we only need a single entry in the table,
		// so we strip the query-string which contains the user-list
		return encodingPrefix + ctx.Request.URL.Path
	}
	return encodingPrefix + ctx.Request.URL.Path + ctx.Request.URL.RawQuery
}
