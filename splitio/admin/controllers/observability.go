package controllers

import (
	"fmt"

	"github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
	pstorage "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
)

type ObservabilityDto struct {
	ActiveSplits   []string       `json:"activeSplits"`
	ActiveSegments map[string]int `json:"activeSegments"`
	ActiveFlagSets []string       `json:"activeFlagSets"`
}

// ObservabilityController interface is used to have a single constructor that returns the apropriate controller
type ObservabilityController interface {
	Register(gin.IRouter)
}

// SyncObservabilityController exposes an observability endpoint exposing cached feature flags & segments information
type SyncObservabilityController struct {
	logger   logging.LoggerInterface
	splits   observability.ObservableSplitStorage
	segments observability.ObservableSegmentStorage
}

// Register mounts the controller endpoints onto the supplied router
func (c *SyncObservabilityController) Register(router gin.IRouter) {
	router.GET("/observability", c.observability)
}

func (c *SyncObservabilityController) observability(ctx *gin.Context) {
	ctx.JSON(200, ObservabilityDto{
		ActiveSplits:   c.splits.SplitNames(),
		ActiveSegments: c.segments.NamesAndCount(),
		ActiveFlagSets: c.splits.GetAllFlagSetNames(),
	})
}

// ProxyObservabilityController exposes an observability endpoint exposing cached feature flags & segments information
type ProxyObservabilityController struct {
	logger    logging.LoggerInterface
	telemetry pstorage.TimeslicedProxyEndpointTelemetry
	splits    observability.ObservableSplitStorage
	segments  observability.ObservableSegmentStorage
}

// Register mounts the controller endpoints onto the supplied router
func (c *ProxyObservabilityController) Register(router gin.IRouter) {
	router.GET("/observability", c.observability)
}

func (c *ProxyObservabilityController) observability(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"activeSplits":            c.splits.SplitNames(),
		"activeSegments":          c.segments.NamesAndCount(),
		"proxyEndpointStats":      c.telemetry.TimeslicedReport(),
		"proxyEndpointStatsTotal": c.telemetry.TotalMetricsReport(),
	})
}

// NewObservabilityController constructs and returns the appropriate struct dependeing on whether the app is split-proxy or split-sync
func NewObservabilityController(proxy bool, logger logging.LoggerInterface, storagePack common.Storages) (ObservabilityController, error) {

	splitStorage, ok := storagePack.SplitStorage.(observability.ObservableSplitStorage)
	if !ok {
		return nil, fmt.Errorf("invalid feature flag storage supplied: %T", storagePack.SplitStorage)
	}

	segmentStorage, ok := storagePack.SegmentStorage.(observability.ObservableSegmentStorage)
	if !ok {
		return nil, fmt.Errorf("invalid segment storage supplied: %T", storagePack.SegmentStorage)
	}

	if !proxy {
		return &SyncObservabilityController{
			logger:   logger,
			splits:   splitStorage,
			segments: segmentStorage,
		}, nil

	}

	telemetry, ok := storagePack.LocalTelemetryStorage.(pstorage.TimeslicedProxyEndpointTelemetry)
	if !ok {
		return nil, fmt.Errorf("invalid local telemetry storage supplied: %T", storagePack.LocalTelemetryStorage)
	}

	return &ProxyObservabilityController{
		logger:    logger,
		splits:    splitStorage,
		segments:  segmentStorage,
		telemetry: telemetry,
	}, nil

}
