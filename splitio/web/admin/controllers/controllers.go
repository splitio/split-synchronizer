package controllers

import (
	"net/http"
	"os"
	"syscall"

	"github.com/splitio/split-synchronizer/conf"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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

// ShowStats returns stats
func ShowStats(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()
	c.JSON(http.StatusOK, gin.H{"counters": counters, "latencies": latencies})
}

// kill process helper
func kill(sig syscall.Signal) error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// StopProccess triggers a kill signal
func StopProccess(c *gin.Context) {
	stopType := c.Param("stopType")
	var toReturn string

	switch stopType {
	case "force":
		toReturn = stopType
		log.PostShutdownMessageToSlack(true)
		defer kill(syscall.SIGKILL)
	case "graceful":
		toReturn = stopType
		defer kill(syscall.SIGINT)
	default:
		c.String(http.StatusBadRequest, "Invalid sign type: %s", toReturn)
		return
	}

	c.String(http.StatusOK, "%s: %s", "Sign has been sent", toReturn)

}

// GetEventsQueueSize returns events queue size
func GetEventsQueueSize(c *gin.Context) {
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := eventsStorageAdapter.Size()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

// GetImpressionsQueueSize returns impressions queue size
func GetImpressionsQueueSize(c *gin.Context) {
	impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := impressionsStorage.Size()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}
