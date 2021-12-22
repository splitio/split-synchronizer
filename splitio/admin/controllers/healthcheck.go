package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services"
)

// HealthCheckController description
type HealthCheckController struct {
	logger              logging.LoggerInterface
	appMonitor          application.MonitorIterface
	dependenciesMonitor services.MonitorIterface
}

func (c *HealthCheckController) appHealth(ctx *gin.Context) {
	status := c.appMonitor.GetHealthStatus()
	if status.Healthy {
		ctx.JSON(http.StatusOK, status)
		return
	}
	ctx.JSON(http.StatusInternalServerError, status)
}

func (c *HealthCheckController) dependenciesHealth(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.dependenciesMonitor.GetHealthStatus())
}

// Register the dashboard endpoints
func (c *HealthCheckController) Register(router gin.IRouter) {
	router.GET("/health/application", c.appHealth)
	router.GET("/health/dependencies", c.dependenciesHealth)
}

// NewHealthCheckController instantiates a new HealthCheck controller
func NewHealthCheckController(
	logger logging.LoggerInterface,
	appMonitor application.MonitorIterface,
	dependenciesMonitor services.MonitorIterface,
) *HealthCheckController {
	return &HealthCheckController{
		logger:              logger,
		appMonitor:          appMonitor,
		dependenciesMonitor: dependenciesMonitor,
	}
}
