package controllers

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/admin/views/dashboard"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
)

// DashboardController contains handlers for rendering the dashboard and its associated FE queries
type DashboardController struct {
	title             string
	proxy             bool
	logger            logging.LoggerInterface
	storages          common.Storages
	httpClients       common.HTTPClients
	layout            *template.Template
	impressionsEvCalc evcalc.Monitor
	eventsEvCalc      evcalc.Monitor
	runtime           common.Runtime
}

// NewDashboardController instantiates a new dashboard controller
func NewDashboardController(
	name string,
	proxy bool,
	logger logging.LoggerInterface,
	storages common.Storages,
	httpClients common.HTTPClients,
	impressionEvCalc evcalc.Monitor,
	eventsEvCalc evcalc.Monitor,
	runtime common.Runtime,
) (*DashboardController, error) {
	toReturn := &DashboardController{
		title:             name,
		proxy:             proxy,
		logger:            logger,
		runtime:           runtime,
		storages:          storages,
		httpClients:       httpClients,
		eventsEvCalc:      eventsEvCalc,
		impressionsEvCalc: impressionEvCalc,
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
	router.GET("/dashboard/health", c.health)
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

// health endpoint returns different health parameters of the app and split service
func (c *DashboardController) health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.gatherHealthInfo())
}

// \} -- end of endpoint functions

func (c *DashboardController) renderDashboard() ([]byte, error) {
	runningMode := "Running as Producer Mode"
	if c.proxy {
		runningMode = "Running as Proxy Mode"
	}

	var layoutBuffer bytes.Buffer
	err := c.layout.Execute(&layoutBuffer, dashboard.DashboardInitializationVars{
		DashboardTitle: c.title,
		RunningMode:    runningMode,
		Version:        splitio.Version,
		ProxyMode:      c.proxy,
		RefreshTime:    10000,
		Stats:          *c.gatherStats(),
		Health:         *c.gatherHealthInfo(),
	})

	if err != nil {
		return nil, fmt.Errorf("error rendering main layout template for dashboard: %w", err)
	}
	return layoutBuffer.Bytes(), nil
}

func (c *DashboardController) gatherStats() *dashboard.GlobalStats {
	var errorMessages []string
	if asHistoricLogger, ok := c.logger.(log.HistoricLogger); ok {
		errorMessages = asHistoricLogger.Messages(logging.LevelError)
	}

	var impCount int64 = 0
	var evCount int64 = 0
	if c.storages.ImpressionStorage != nil && c.storages.EventStorage != nil {
		impCount = c.storages.ImpressionStorage.Count()
		evCount = c.storages.EventStorage.Count()
	}

	return &dashboard.GlobalStats{
		Splits:                 bundleSplitInfo(c.storages.SplitStorage),
		Segments:               bundleSegmentInfo(c.storages.SplitStorage, c.storages.SegmentStorage),
		Latencies:              bundleProxyLatencies(c.storages.LocalTelemetryStorage),
		BackendLatencies:       bundleLocalSyncLatencies(c.storages.LocalTelemetryStorage),
		ImpressionsQueueSize:   impCount,
		EventsQueueSize:        evCount,
		ImpressionsLambda:      c.impressionsEvCalc.Lambda(),
		EventsLambda:           c.eventsEvCalc.Lambda(),
		RequestsOk:             0, // TODO
		RequestsErrored:        0, // TODO
		SdksTotalRequests:      0, // TODO
		BackendRequestsOk:      0, // TODO
		BackendRequestsErrored: 0, // TODO
		BackendTotalRequests:   0, // TODO
		LoggedErrors:           0, // TODO
		LoggedMessages:         errorMessages,
		Uptime:                 int64(c.runtime.Uptime().Seconds()),
	}
}

func (c *DashboardController) gatherHealthInfo() *dashboard.Health {
	// TODO(sanzamauro): Populate this accordingly
	return &dashboard.Health{
		SDKServerStatus:   false,
		EventServerStatus: false,
		AuthServerStatus:  false,
		StorageStatus:     false,
		HealthySince:      0,
	}
}
