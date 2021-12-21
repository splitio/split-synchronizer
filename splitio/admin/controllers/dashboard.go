package controllers

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/admin/views/dashboard"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/log"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
)

// DashboardController contains handlers for rendering the dashboard and its associated FE queries
type DashboardController struct {
	title             string
	proxy             bool
	logger            logging.LoggerInterface
	storages          adminCommon.Storages
	layout            *template.Template
	impressionsEvCalc evcalc.Monitor
	eventsEvCalc      evcalc.Monitor
	runtime           common.Runtime
	appMonitor        application.MonitorIterface
}

// NewDashboardController instantiates a new dashboard controller
func NewDashboardController(
	name string,
	proxy bool,
	logger logging.LoggerInterface,
	storages adminCommon.Storages,
	impressionEvCalc evcalc.Monitor,
	eventsEvCalc evcalc.Monitor,
	runtime common.Runtime,
	appMonitor application.MonitorIterface,
) (*DashboardController, error) {

	toReturn := &DashboardController{
		title:             name,
		proxy:             proxy,
		logger:            logger,
		runtime:           runtime,
		storages:          storages,
		eventsEvCalc:      eventsEvCalc,
		impressionsEvCalc: impressionEvCalc,
		appMonitor:        appMonitor,
	}

	var err error
	toReturn.layout, err = dashboard.AssembleDashboardTemplate()
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate Main template: %w", err)
	}
	return toReturn, nil
}

// Register the dashboard endpoints
func (c *DashboardController) Register(router gin.IRouter) {
	router.GET("/dashboard", c.dashboard)
	router.GET("/dashboard/segmentKeys/:segment", c.segmentKeys)
	router.GET("/dashboard/stats", c.stats)
}

// Endpoint functions \{

// dashboard returns a dashboard
func (c *DashboardController) dashboard(ctx *gin.Context) {
	dashboard, err := c.renderDashboard()
	if err != nil {
		c.logger.Error("error rendering dashboard: ", err)
		ctx.AbortWithStatus(500)
		return
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	ctx.Writer.Write(dashboard)
}

// stats returns stats for dashboard
func (c *DashboardController) stats(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.gatherStats())
}

// segmentKeys returns a keys for a given segment
func (c *DashboardController) segmentKeys(ctx *gin.Context) {
	segmentName := ctx.Param("segment")
	if segmentName == "" {
		ctx.AbortWithStatus(400)
		return
	}
	ctx.JSON(200, bundleSegmentKeysInfo(segmentName, c.storages.SegmentStorage))
}

// \} -- end of endpoint functions

func (c *DashboardController) renderDashboard() ([]byte, error) {
	var layoutBuffer bytes.Buffer
	err := c.layout.Execute(&layoutBuffer, dashboard.RootObject{
		DashboardTitle: c.title,
		Version:        splitio.Version,
		ProxyMode:      c.proxy,
		RefreshTime:    30000,
		Stats:          *c.gatherStats(),
		Health:         c.appMonitor.GetHealthStatus(),
	})

	if err != nil {
		return nil, fmt.Errorf("error rendering main layout template for dashboard: %w", err)
	}
	return layoutBuffer.Bytes(), nil
}

func (c *DashboardController) gatherStats() *dashboard.GlobalStats {
	var errorMessages []string
	var errorCount int64
	if asHistoricLogger, ok := c.logger.(log.HistoricLogger); ok {
		errorMessages = asHistoricLogger.Messages(logging.LevelError)
		errorCount = asHistoricLogger.TotalCount(logging.LevelError)
	}

	upstreamOkReqs, upstreamErrorReqs := getUpstreamRequestCount(c.storages.LocalTelemetryStorage)
	proxyOkReqs, proxyErrorReqs := getProxyRequestCount(c.storages.LocalTelemetryStorage)

	impressionsLambda := float64(0)
	if c.impressionsEvCalc != nil {
		impressionsLambda = c.impressionsEvCalc.Lambda()
	}

	eventsLambda := float64(0)
	if c.eventsEvCalc != nil {
		eventsLambda = c.eventsEvCalc.Lambda()
	}

	return &dashboard.GlobalStats{
		Splits:                 bundleSplitInfo(c.storages.SplitStorage),
		Segments:               bundleSegmentInfo(c.storages.SplitStorage, c.storages.SegmentStorage),
		Latencies:              bundleProxyLatencies(c.storages.LocalTelemetryStorage),
		BackendLatencies:       bundleLocalSyncLatencies(c.storages.LocalTelemetryStorage),
		ImpressionsQueueSize:   getImpressionSize(c.storages.ImpressionStorage),
		EventsQueueSize:        getEventsSize(c.storages.EventStorage),
		ImpressionsLambda:      impressionsLambda,
		EventsLambda:           eventsLambda,
		RequestsOk:             proxyOkReqs,
		RequestsErrored:        proxyErrorReqs,
		SdksTotalRequests:      proxyOkReqs + proxyErrorReqs,
		BackendRequestsOk:      upstreamOkReqs,
		BackendRequestsErrored: upstreamErrorReqs,
		BackendTotalRequests:   upstreamOkReqs + upstreamErrorReqs,
		LoggedErrors:           errorCount,
		LoggedMessages:         errorMessages,
		Uptime:                 int64(c.runtime.Uptime().Seconds()),
	}
}
