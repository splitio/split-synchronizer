package controllers

import (
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"

	tmw "github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/tasks"
)

// TelemetryServerController bundles all request handler for sdk-server apis
type TelemetryServerController struct {
	logger     logging.LoggerInterface
	telemetry  storage.ProxyEndpointTelemetry
	configSink tasks.DeferredRecordingTask
	usageSink  tasks.DeferredRecordingTask
}

// NewTelemetryServerController returns a new events server controller
func NewTelemetryServerController(
	logger logging.LoggerInterface,
	telemetry storage.ProxyEndpointTelemetry,
	configSync tasks.DeferredRecordingTask,
	usageSync tasks.DeferredRecordingTask,
) *TelemetryServerController {
	return &TelemetryServerController{
		logger:     logger,
		telemetry:  telemetry,
		configSink: configSync,
		usageSink:  usageSync,
	}
}

// Config endpoint accepts telemtetry config objects
func (c *TelemetryServerController) Config(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.TelemetryConfigEndpoint)
	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		c.telemetry.IncrEndpointStatus(storage.TelemetryConfigEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	err = c.configSink.Stage(internal.NewRawTelemetryConfig(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Config telemetry queue queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push config telemetry into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusOK, nil)
	c.telemetry.IncrEndpointStatus(storage.TelemetryConfigEndpoint, http.StatusOK)
}

// Usage endpoint accepts telemtetry config objects
func (c *TelemetryServerController) Usage(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.TelemetryRuntimeEndpoint)
	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		c.telemetry.IncrEndpointStatus(storage.TelemetryRuntimeEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	err = c.usageSink.Stage(internal.NewRawTelemetryUsage(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Usage telemetry queue queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push usage telemetry into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusOK, nil)
	c.telemetry.IncrEndpointStatus(storage.TelemetryRuntimeEndpoint, http.StatusOK)
}
