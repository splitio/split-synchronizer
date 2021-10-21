package controllers

import (
	"net/http"

	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/common"

	"github.com/gin-gonic/gin"
)

// InfoController contains handlers for system information purposes
type InfoController struct {
	proxy          bool
	cfg            *conf.ConfigData
	localTelemetry storage.TelemetryPeeker
	runtime        common.Runtime
}

// NewInfoController constructs a new InfoController to be mounted on a gin router
func NewInfoController(proxy bool, runtime common.Runtime, localTelemetry storage.TelemetryPeeker) (*InfoController, error) {
	return &InfoController{
		proxy:          proxy,
		cfg:            &conf.Data, // TODO(mredolatti): accept this from a parameter
		localTelemetry: localTelemetry,
		runtime:        runtime,
	}, nil
}

// Uptime returns the service uptime
func (c *InfoController) Uptime(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"uptime": c.runtime.Uptime()})
}

// Version returns the service version
func (c *InfoController) Version(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"version": splitio.Version})
}

// Ping returns a 200 HTTP status code
func (c *InfoController) Ping(ctx *gin.Context) {
	ctx.String(http.StatusOK, "%s", "pong")
}

// ShowStats returns stats
func (c *InfoController) ShowStats(ctx *gin.Context) {
	httpErrors := map[string]map[int]int{
		"splitChanges":    c.localTelemetry.PeekHTTPErrors(telemetry.SplitSync),
		"segmentChanges":  c.localTelemetry.PeekHTTPErrors(telemetry.SegmentSync),
		"impressions":     c.localTelemetry.PeekHTTPErrors(telemetry.ImpressionSync),
		"impressionCount": c.localTelemetry.PeekHTTPErrors(telemetry.ImpressionCountSync),
		"events":          c.localTelemetry.PeekHTTPErrors(telemetry.EventSync),
		"telemetry":       c.localTelemetry.PeekHTTPErrors(telemetry.TelemetrySync),
	}

	httpLatencies := map[string][]int64{
		"splitChanges":    c.localTelemetry.PeekHTTPLatencies(telemetry.SplitSync),
		"segmentChanges":  c.localTelemetry.PeekHTTPLatencies(telemetry.SegmentSync),
		"impressions":     c.localTelemetry.PeekHTTPLatencies(telemetry.ImpressionSync),
		"impressionCount": c.localTelemetry.PeekHTTPLatencies(telemetry.ImpressionCountSync),
		"events":          c.localTelemetry.PeekHTTPLatencies(telemetry.EventSync),
		"telemetry":       c.localTelemetry.PeekHTTPLatencies(telemetry.TelemetrySync),
	}

	ctx.JSON(http.StatusOK, gin.H{"errors": httpErrors, "latencies": httpLatencies})
}

// GetConfiguration Returns Sync Config
func (c *InfoController) GetConfiguration(ctx *gin.Context) {
	config := map[string]interface{}{
		"mode":      nil,
		"redisMode": nil,
		"redis":     nil,
		"proxy":     nil,
	}
	if c.proxy {
		config["mode"] = "ProxyMode"
		config["proxy"] = c.cfg.Proxy
	} else {
		config["mode"] = "ProducerMode"
		config["redisMode"] = redisModeStr(&(c.cfg.Redis))
		config["redis"] = c.cfg.Redis
	}
	ctx.JSON(http.StatusOK, gin.H{
		"apiKey":              logging.ObfuscateAPIKey(conf.Data.APIKey),
		"impressionListener":  c.cfg.ImpressionListener,
		"splitRefreshRate":    c.cfg.SplitsFetchRate,
		"segmentsRefreshRate": c.cfg.SegmentFetchRate,
		"impressionsPostRate": c.cfg.ImpressionsPostRate,
		"impressionsPerPost":  c.cfg.ImpressionsPerPost,
		"impressionsThreads":  c.cfg.ImpressionsThreads,
		"impressionsMode":     c.cfg.ImpressionsMode,
		"eventsPostRate":      c.cfg.EventsPostRate,
		"eventsPerPost":       c.cfg.EventsPerPost,
		"eventsThreads":       c.cfg.EventsThreads,
		"metricsPostRate":     c.cfg.MetricsPostRate,
		"httpTimeout":         c.cfg.HTTPTimeout,
		"mode":                config["mode"],
		"redisMode":           config["redisMode"],
		"log":                 c.cfg.Logger,
		"redis":               config["redis"],
		"proxy":               config["proxy"],
		"admin":               c.cfg.Producer.Admin,
	})
}

func redisModeStr(redisCfg *conf.RedisSection) string {
	if redisCfg.ClusterMode {
		return "Cluster"
	}
	if redisCfg.SentinelReplication {
		return "Sentinel"
	}
	return "Standard"
}
