package controllers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
)

// TelemetryServerController bundles all request handler for sdk-server apis
type TelemetryServerController struct {
	logger             logging.LoggerInterface
	configSink         tasks.DeferredRecordingTask
	usageSink          tasks.DeferredRecordingTask
	keysClientSideSink tasks.DeferredRecordingTask
	keysServerSideSink tasks.DeferredRecordingTask
	apikeyValidator    func(string) bool
}

// NewTelemetryServerController returns a new events server controller
func NewTelemetryServerController(
	logger logging.LoggerInterface,
	configSync tasks.DeferredRecordingTask,
	usageSync tasks.DeferredRecordingTask,
	keysClientSideSink tasks.DeferredRecordingTask,
	keysServerSideSink tasks.DeferredRecordingTask,
	apikeyValidator func(string) bool,
) *TelemetryServerController {
	return &TelemetryServerController{
		logger:             logger,
		configSink:         configSync,
		keysClientSideSink: keysClientSideSink,
		keysServerSideSink: keysServerSideSink,
		usageSink:          usageSync,
		apikeyValidator:    apikeyValidator,
	}
}

// Register mounts telemetry-related endpoints onto the supplied router
func (c *TelemetryServerController) Register(router gin.IRouter, beacon gin.IRouter) {
	router.POST("/metrics/config", c.Config)
	router.POST("/v1/metrics/config", c.Config)
	router.POST("/metrics/usage", c.Usage)
	router.POST("/v1/metrics/usage", c.Usage)
	router.POST("/keys/cs", c.keysClientSide)
	router.POST("/v1/keys/cs", c.keysClientSide)
	router.POST("/keys/ss", c.keysServerSide)
	router.POST("/v1/keys/ss", c.keysServerSide)

	// beacon endpoints
	beacon.POST("/metrics/usage/beacon", c.UsageBeacon)
	beacon.POST("/v1/metrics/usage/beacon", c.UsageBeacon)
	beacon.POST("/keys/cs/beacon", c.keysClientSideBeacon)
	beacon.POST("/v1/keys/cs/beacon", c.keysClientSideBeacon)
}

// Config endpoint accepts telemtetry config objects
func (c *TelemetryServerController) Config(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
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
}

// Usage endpoint accepts telemtetry config objects
func (c *TelemetryServerController) Usage(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	err = c.usageSink.Stage(internal.NewRawTelemetryUsage(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Usage telemetry queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push usage telemetry into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusOK, nil)
}

func (c *TelemetryServerController) UsageBeacon(ctx *gin.Context) {
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	var body beaconMessage
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	if !c.apikeyValidator(body.Token) {
		ctx.AbortWithStatus(401)
		return
	}

	code := http.StatusNoContent

	err = c.usageSink.Stage(internal.NewRawTelemetryUsage(dtos.Metadata{SDKVersion: body.Sdk, MachineIP: "NA", MachineName: "NA"}, body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Usage telemetry queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push usage telemetry into the staging queue")
		}
		return
	}

	ctx.JSON(code, nil)
}

func (c *TelemetryServerController) keysClientSide(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body in keys/cs endpoint: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	code := http.StatusOK
	err = c.keysClientSideSink.Stage(internal.NewRawTelemetryKeysClientSide(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Keys Client Side queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push keys Client Side into the staging queue")
		}
		return
	}
	ctx.JSON(code, nil)
}

func (c *TelemetryServerController) keysClientSideBeacon(ctx *gin.Context) {
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	var body beaconMessage
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	if !c.apikeyValidator(body.Token) {
		ctx.AbortWithStatus(401)
		return
	}

	code := http.StatusNoContent

	err = c.keysClientSideSink.Stage(internal.NewRawTelemetryKeysClientSide(dtos.Metadata{SDKVersion: body.Sdk, MachineIP: "NA", MachineName: "NA"}, body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Keys Client Side queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push keys client side into the staging queue")
		}
		return
	}

	ctx.JSON(code, nil)
}

func (c *TelemetryServerController) keysServerSide(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body in keys/ss endpoint: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	code := http.StatusOK
	err = c.keysServerSideSink.Stage(internal.NewRawTelemetryKeysServerSide(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Keys Server Side queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push keys Server Side into the staging queue")
		}
		return
	}
	ctx.JSON(code, nil)
}
