package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
)

// EndpointKey is used to set the endpoint for latency tracker within the request handler
const EndpointKey = "ep"

// Endpoint paths
const (
	pathSplitChanges           = "/api/splitChanges"
	pathSegmentChanges         = "/api/segmentChanges"
	pathMySegments             = "/api/mySegments"
	pathImpressionsBulk        = "/api/testImpressions/bulk"
	pathImpressionsCount       = "/api/testImpressions/count"
	pathImpressionsBulkBeacon  = "/api/testImpressions/beacon"
	pathImpressionsCountBeacon = "/api/testImpressions/count/beacon"
	pathEventsBulk             = "/api/events/bulk"
	pathEventsBeacon           = "/api/events/beacon"
	pathTelemetryConfig        = "/api/metrics/config"
	pathTelemetryUsage         = "/api/metrics/usage"
	pathAuth                   = "/api/auth"
	pathAuthV2                 = "/api/auth/v2"
)

// SetEndpoint stores the endpoint in the context for future middleware querying
func SetEndpoint(ctx *gin.Context) {
	switch path := ctx.Request.URL.Path; path {
	case pathSplitChanges:
		ctx.Set(EndpointKey, storage.SplitChangesEndpoint)
	case pathImpressionsBulk:
		ctx.Set(EndpointKey, storage.ImpressionsBulkEndpoint)
	case pathImpressionsCount:
		ctx.Set(EndpointKey, storage.ImpressionsCountEndpoint)
	case pathImpressionsBulkBeacon:
		ctx.Set(EndpointKey, storage.ImpressionsBulkBeaconEndpoint)
	case pathImpressionsCountBeacon:
		ctx.Set(EndpointKey, storage.ImpressionsCountBeaconEndpoint)
	case pathEventsBulk:
		ctx.Set(EndpointKey, storage.EventsBulkEndpoint)
	case pathEventsBeacon:
		ctx.Set(EndpointKey, storage.EventsBulkBeaconEndpoint)
	case pathTelemetryConfig:
		ctx.Set(EndpointKey, storage.TelemetryConfigEndpoint)
	case pathTelemetryUsage:
		ctx.Set(EndpointKey, storage.TelemetryRuntimeEndpoint)
	case pathAuth, pathAuthV2:
		ctx.Set(EndpointKey, storage.AuthEndpoint)
	default:
		if strings.HasPrefix(path, pathSplitChanges) {
			ctx.Set(EndpointKey, storage.SplitChangesEndpoint)
		} else if strings.HasPrefix(path, pathSegmentChanges) {
			ctx.Set(EndpointKey, storage.SegmentChangesEndpoint)
		} else if strings.HasPrefix(path, pathMySegments) {
			ctx.Set(EndpointKey, storage.MySegmentsEndpoint)
		}
	}
}
