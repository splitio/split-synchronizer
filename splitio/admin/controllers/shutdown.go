package controllers

import (
	"net/http"
	"os"
	"syscall"

	"github.com/gin-gonic/gin"
)

const (
	forcedShutdown   = "force"
	gracefulShutdown = "graceful"
)

// ShutdownController bundles handlers that can shut down the synchronizer app
type ShutdownController struct{}

// StopProcess handles requests to shut down the synchronizer app
func (c *ShutdownController) StopProcess(ctx *gin.Context) {
	stopType := ctx.Param("stopType")
	var toReturn string

	switch stopType {
	case forcedShutdown:
		toReturn = stopType
		// log.PostShutdownMessageToSlack(true)
		defer kill(syscall.SIGKILL)
	case gracefulShutdown:
		toReturn = stopType
		defer kill(syscall.SIGINT)
	default:
		ctx.String(http.StatusBadRequest, "Invalid sign type: %s", toReturn)
		return
	}

	ctx.String(http.StatusOK, "%s: %s", "Signal has been sent", toReturn)

}

// kill process helper
func kill(sig syscall.Signal) error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(sig)
}
