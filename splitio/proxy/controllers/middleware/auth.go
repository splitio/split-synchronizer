package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyValidator is a small component that validates apikeys
type APIKeyValidator struct {
	apikeys map[string]struct{}
}

// NewAPIKeyValidator instantiates an apikey validation component
func NewAPIKeyValidator(apikeys []string) *APIKeyValidator {
	toRet := &APIKeyValidator{apikeys: make(map[string]struct{})}
	for _, key := range apikeys {
		toRet.apikeys[key] = struct{}{}
	}

	return toRet
}

// IsValid checks if an apikey is valid
func (v *APIKeyValidator) IsValid(apikey string) bool {
	_, ok := v.apikeys[apikey]
	return ok
}

// AsMiddleware is a function to be used as a gin middleware
func (v *APIKeyValidator) AsMiddleware(ctx *gin.Context) {
	auth := strings.Split(ctx.Request.Header.Get("Authorization"), " ")
	if len(auth) != 2 || auth[0] != "Bearer" {
		ctx.AbortWithStatus(401)
		return
	}

	if !v.IsValid(auth[1]) {
		ctx.AbortWithStatus(401)
	}
}
