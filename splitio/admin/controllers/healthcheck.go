package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services"
)

// HealthCheckController description
type HealthCheckController struct {
	logger          logging.LoggerInterface
	appMonitor      *application.MonitorImp
	servicesMonitor *services.MonitorImp
}

func (c *HealthCheckController) appHealth(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.appMonitor.GetHealthStatus())
}

func (c *HealthCheckController) servicesHealth(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.servicesMonitor.GetHealthStatus())
}

// Register the dashboard endpoints
func (c *HealthCheckController) Register(router gin.IRouter) {
	router.GET("/health/application", c.appHealth)
	router.GET("/health/services", c.servicesHealth)
}

// NewHealthCheckController instantiates a new HealthCheck controller
func NewHealthCheckController(
	logger logging.LoggerInterface,
	appMonitor *application.MonitorImp,
	servicesMonitor *services.MonitorImp,
) *HealthCheckController {
	return &HealthCheckController{
		logger:          logger,
		appMonitor:      appMonitor,
		servicesMonitor: servicesMonitor,
	}
}
