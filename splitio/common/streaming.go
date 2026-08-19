package common

import "github.com/splitio/go-toolkit/v5/logging"

// LogStreamingProtocol emits an info line describing which HTTP protocol the
// streaming (SSE) connection is configured to use. It reflects the configured
// intent (the streaming-force-http1 flag), not the protocol negotiated on the
// wire. When streaming is disabled there is no SSE connection, so nothing is
// logged.
func LogStreamingProtocol(logger logging.LoggerInterface, streamingEnabled bool, forceHTTP1 bool) {
	if !streamingEnabled {
		return
	}
	if forceHTTP1 {
		logger.Info("Streaming (SSE) connection configured to use HTTP/1.1")
		return
	}
	logger.Info("Streaming (SSE) connection configured to use HTTP/2")
}
