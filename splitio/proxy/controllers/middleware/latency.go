package middleware

import (
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/gin-gonic/gin"
)

// MetricsMiddleware is meant to be used for capturing endpoint latencies and return status codes
type MetricsMiddleware struct {
	tracker storage.ProxyEndpointTelemetry
}

// NewProxyMetricsMiddleware instantiates a new local-telemetry tracking middleware
func NewProxyMetricsMiddleware(lats storage.ProxyEndpointTelemetry) *MetricsMiddleware {
	return &MetricsMiddleware{tracker: lats}
}

// Track is the function to be invoked for every request being handled
func (m *MetricsMiddleware) Track(ctx *gin.Context) {
	before := time.Now()
	ctx.Next()
	endpoint, exists := ctx.Get(EndpointKey)
	if asInt, ok := endpoint.(int); exists && ok {
		m.tracker.RecordEndpointLatency(asInt, time.Now().Sub(before))
		m.tracker.IncrEndpointStatus(asInt, ctx.Writer.Status())
	}
}
