package caching

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/gincache"
	"github.com/splitio/go-split-commons/v4/dtos"
)

const (
	// SurrogateContextKey is the gin context key used to store surrogates generated on each response
	SurrogateContextKey = "surrogates"

	// StickyContextKey should be set to (boolean) true whenever we want an entry to be kept in cache when making room
	// for new entries
	StickyContextKey = gincache.StickyEntry

	// SplitSurrogate key (we only need one, since all splitChanges should be expired when an update is processed)
	SplitSurrogate = "sp"

	// AuthSurrogate key (having push disabled, it's safe to cache this and return it on all requests)
	AuthSurrogate = "au"

	cacheSize = 1000000
)

const segmentPrefix = "se::"

// MakeSurrogateForSegmentChanges creates a surrogate key for the segment being queried
func MakeSurrogateForSegmentChanges(segmentName string) string {
	return segmentPrefix + segmentName
}

// MakeSurrogateForMySegments creates a list surrogate keys for all the segments involved
func MakeSurrogateForMySegments(mysegments []dtos.MySegmentDTO) []string {
	if len(mysegments) == 0 {
		return nil
	}

	surrogates := make([]string, 0, len(mysegments))
	for idx := range mysegments {
		surrogates = append(surrogates, segmentPrefix+mysegments[idx].Name)
	}
	return surrogates
}

// MakeProxyCache creates and configures a split-proxy-ready cache
func MakeProxyCache() *gincache.Middleware {
	return gincache.New(&gincache.Options{
		SuccessfulOnly: true, // we're not interested in caching non-200 responses
		Size:           cacheSize,
		KeyFactory: func(ctx *gin.Context) string {
			if strings.HasPrefix(ctx.Request.URL.Path, "/api/auth") || strings.HasPrefix(ctx.Request.URL.Path, "/api/v2/auth") {
				// For auth requests, since we don't support streaming yet, we only need a single entry in the table,
				// so we strip the query-string which contains the user-list
				return ctx.Request.URL.Path
			}
			return ctx.Request.URL.Path + ctx.Request.URL.RawQuery
		},
		// we make each request handler responsible for generating the surrogates.
		// this way we can use segment names as surrogates for mysegments & segment changes
		// with a lot less work
		SurrogateFactory: func(ctx *gin.Context) []string { return ctx.GetStringSlice(SurrogateContextKey) },
	})
}
