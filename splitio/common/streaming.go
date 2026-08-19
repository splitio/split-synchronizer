package common

import "github.com/splitio/go-toolkit/v5/logging"

// LogStreamingForceHTTP1 logs an info line when the streaming (SSE) connection
// is pinned to HTTP/1.1 via the streaming-force-http1 setting.
func LogStreamingForceHTTP1(logger logging.LoggerInterface, streamingEnabled bool, forceHTTP1 bool) {
	if streamingEnabled && forceHTTP1 {
		logger.Info("Streaming (SSE) connection forced to HTTP/1.1")
	}
}
