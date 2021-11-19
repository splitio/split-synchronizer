package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
)

const (
	forcedShutdown   = "force"
	gracefulShutdown = "graceful"
)

// ShutdownController bundles handlers that can shut down the synchronizer app
type ShutdownController struct {
	runtime common.Runtime
}

// NewShutdownController instantiates a shutdown request handling controller
func NewShutdownController(runtime common.Runtime) *ShutdownController {
	return &ShutdownController{runtime: runtime}
}

// Register mounts the endpoints
func (c *ShutdownController) Register(router gin.IRouter) {
	router.GET("/stop/:stopType", c.stopProcess)
}

func (c *ShutdownController) stopProcess(ctx *gin.Context) {
	stopType := ctx.Param("stopType")
	var toReturn string

	switch stopType {
	case forcedShutdown:
		toReturn = stopType
		c.runtime.Kill()
	case gracefulShutdown:
		c.runtime.Shutdown()
	default:
		ctx.String(http.StatusBadRequest, "Invalid sign type: %s", toReturn)
		return
	}

	ctx.String(http.StatusOK, "%s: %s", "Signal has been sent", toReturn)

}
