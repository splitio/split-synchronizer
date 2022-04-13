package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
)

// APIKeyValidator is a small component that validates apikeys
type APIKeyValidator struct {
	apikeys map[string]struct{}
	tracker storage.EndpointStatusCodeProducer
}

// NewAPIKeyValidator instantiates an apikey validation component
func NewAPIKeyValidator(apikeys []string, statusCodeTracker storage.EndpointStatusCodeProducer) *APIKeyValidator {
	toRet := &APIKeyValidator{
		apikeys: make(map[string]struct{}),
		tracker: statusCodeTracker,
	}

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
	if len(auth) != 2 || auth[0] != "Bearer" || !v.IsValid(auth[1]) {
		endpoint, exists := ctx.Get(EndpointKey)
		if asInt, ok := endpoint.(int); exists && ok {
			v.tracker.IncrEndpointStatus(asInt, 401)
		}
		ctx.AbortWithStatus(401)
	}
}
