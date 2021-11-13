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

// StopProcess handles requests to shut down the synchronizer app
func (c *ShutdownController) StopProcess(ctx *gin.Context) {
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
