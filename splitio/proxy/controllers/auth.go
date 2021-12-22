package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AuthServerController bundles all request handler for sdk-server apis
type AuthServerController struct{}

// NewAuthServerController instantiates a new sdk server controller
func NewAuthServerController() *AuthServerController {
	return &AuthServerController{}
}

// Register mounts the sdk-server endpoints onto the supplied router
func (c *AuthServerController) Register(router gin.IRouter) {
	router.GET("/auth", c.AuthV1)
	router.GET("/v2/auth", c.AuthV1)
}

// AuthV1 always returns pushEnabled = false and no token
func (c *AuthServerController) AuthV1(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"pushEnabled": false, "token": ""})
}
