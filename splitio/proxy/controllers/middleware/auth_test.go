package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockEndpointTracker struct {
	incrEndpointStatusCall func(edpoint int, status int)
}

func (m *mockEndpointTracker) IncrEndpointStatus(endpoint int, status int) {
	m.incrEndpointStatusCall(endpoint, status)
}

func TestAuthMiddleWare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	authMW := NewAPIKeyValidator([]string{"apikey1", "apikey2"}, &mockEndpointTracker{incrEndpointStatusCall: func(int, int) {}})

	router.GET("/api/test", authMW.AsMiddleware, func(ctx *gin.Context) {})

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/test", nil)
	ctx.Request.Header.Set("Authorization", "Bearer apikey1")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	resp = httptest.NewRecorder()
	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/test", nil)
	ctx.Request.Header.Set("Authorization", "Bearer apikey2")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 200 {
		t.Error("Status code should be 200 and is ", resp.Code)
	}

	resp = httptest.NewRecorder()
	ctx.Request, _ = http.NewRequest(http.MethodGet, "/api/test", nil)
	ctx.Request.Header.Set("Authorization", "Bearer apikey3")
	router.ServeHTTP(resp, ctx.Request)
	if resp.Code != 401 {
		t.Error("Status code should be 401 and is ", resp.Code)
	}
}
