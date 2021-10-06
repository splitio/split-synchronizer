package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
)

// EndpointKey is used to set the endpoint for latency tracker within the request handler
const EndpointKey = "ep"

// LatencyMiddleware is meant to be used for capturing endpoint latencies
type LatencyMiddleware struct {
	tracker storage.ProxyEndpointLatencies
}

// NewProxyLatencyMiddleware instantiates a new latency tracking middleware
func NewProxyLatencyMiddleware(lats storage.ProxyEndpointLatencies) *LatencyMiddleware {
	return &LatencyMiddleware{tracker: lats}
}

// Track is the function to be invoked for every request being handled
func (m *LatencyMiddleware) Track(c *gin.Context) {
	before := time.Now()
	c.Next()
	endpoint, exists := c.Get(EndpointKey)
	if asInt, ok := endpoint.(int); exists && ok {
		m.tracker.RecordEndpointLatency(asInt, time.Now().Sub(before))
	}
}
