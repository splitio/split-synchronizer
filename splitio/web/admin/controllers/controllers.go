package controllers

import (
	"net/http"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/stats"
)

// Uptime returns the service uptime
func Uptime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uptime": stats.UptimeFormated()})
}

// Version returns the service version
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": splitio.Version})
}

// Ping returns a 200 HTTP status code
func Ping(c *gin.Context) {
	c.String(http.StatusOK, "%s", "pong")
}

// StopProccess triggers a kill signal
func StopProccess(c *gin.Context) {
	stopType := c.Param("stopType")
	var toReturn string

	switch stopType {
	case "force":
		toReturn = stopType
		defer syscall.Kill(syscall.Getpid(), syscall.SIGKILL)
	case "graceful":
		toReturn = stopType
		defer syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	default:
		c.String(http.StatusBadRequest, "Invalid sign type: %s", toReturn)
		return
	}

	c.String(http.StatusOK, "%s: %s", "Sign has been sent", toReturn)

}
