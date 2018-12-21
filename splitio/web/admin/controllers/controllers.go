package controllers

import (
	"net/http"
	"os"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
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

// GetConfiguration Returns Sync Config
func GetConfiguration(c *gin.Context) {
	config := map[string]interface{}{
		"mode":                      nil,
		"redisMode":                 nil,
		"legacyImpressionsFetching": nil,
	}
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		config["mode"] = "ProxyMode"
	} else {
		config["mode"] = "ProducerMode"
		if conf.Data.Redis.ClusterMode {
			config["redisMode"] = "Cluster"
		} else {
			if conf.Data.Redis.SentinelReplication {
				config["redisMode"] = "Sentinel"
			} else {
				config["redisMode"] = "Simple"
			}
		}
		config["legacyImpressionsFetching"] = conf.Data.Redis.LegacyImpressionsFetching
	}
	c.JSON(http.StatusOK, gin.H{
		"splitRefreshRate":          conf.Data.SplitsFetchRate,
		"segmentsRefreshRate":       conf.Data.SegmentFetchRate,
		"impressionsRefreshRate":    conf.Data.ImpressionsPostRate,
		"impressionsPerPost":        conf.Data.ImpressionsPerPost,
		"impressionsThreads":        conf.Data.ImpressionsThreads,
		"eventsPushRate":            conf.Data.EventsPushRate,
		"eventsConsumerReadSize":    conf.Data.EventsConsumerReadSize,
		"eventsConsumerThreads":     conf.Data.EventsConsumerThreads,
		"metricsRefreshRate":        conf.Data.MetricsPostRate,
		"httpTimeout":               conf.Data.HTTPTimeout,
		"mode":                      config["mode"],
		"redisMode":                 config["redisMode"],
		"legacyImpressionsFetching": config["legacyImpressionsFetching"],
	})
}
