package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
)

func TestLatencyMiddleWare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)

	tStorage := storage.NewProxyTelemetryFacade()
	tMw := NewProxyMetricsMiddleware(tStorage)

	router.GET("/api/test", tMw.Track, func(ctx *gin.Context) { ctx.Set(EndpointKey, storage.ImpressionsBulkEndpoint) })

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/test", nil)
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	occurrences := int64(0)
	for _, i := range tStorage.PeekEndpointLatency(storage.ImpressionsBulkEndpoint) {
		occurrences += i
	}
	if occurrences != 1 {
		t.Error("there should be one latency recorded for impressions bulk posting")
	}
}
