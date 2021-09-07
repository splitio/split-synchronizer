package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/common/impressionlistener"
	tmw "github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/tasks"
)

// EventsServerController bundles all request handler for sdk-server apis
type EventsServerController struct {
	logger              logging.LoggerInterface
	telemetry           storage.ProxyEndpointTelemetry
	impressionsSink     tasks.DeferredRecordingTask
	impressionCountSink tasks.DeferredRecordingTask
	eventsSink          tasks.DeferredRecordingTask
	listener            impressionlistener.ImpressionBulkListener
	apikeyValidator     func(*string) bool
}

// NewEventsServerController returns a new events server controller
func NewEventsServerController(
	logger logging.LoggerInterface,
	telemetry storage.ProxyEndpointTelemetry,
	impressionsSink tasks.DeferredRecordingTask,
	impressionCountSink tasks.DeferredRecordingTask,
	eventsSink tasks.DeferredRecordingTask,
	listener impressionlistener.ImpressionBulkListener,
	apikeyValidator func(*string) bool,
) *EventsServerController {
	return &EventsServerController{
		logger:              logger,
		telemetry:           telemetry,
		impressionsSink:     impressionsSink,
		impressionCountSink: impressionCountSink,
		eventsSink:          eventsSink,
		listener:            listener,
		apikeyValidator:     apikeyValidator,
	}
}

// TestImpressionsBulk endpoint accepts impression bulks
func (c *EventsServerController) TestImpressionsBulk(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.ImpressionsBulkEndpoint)
	metadata := metadataFromHeaders(ctx)
	impressionsMode := parseImpressionsMode(ctx.Request.Header.Get("SplitSDKImpressionsMode"))
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}
	if c.listener != nil {
		err = c.listener.Submit(data, &metadata)
	}

	err = c.impressionsSink.Stage(internal.NewRawImpressions(metadata, impressionsMode, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Impressions queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push impressions into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusOK, nil)
	c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkEndpoint, http.StatusOK)
}

// TestImpressionsBeacon accepts beacon style posts with impressions payload
func (c *EventsServerController) TestImpressionsBeacon(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.ImpressionsBulkBeaconEndpoint)
	if ctx.Request.Body == nil {
		c.logger.Error("Nil body when testImpressions/beacon request.")

		c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkBeaconEndpoint, http.StatusBadRequest)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading testImpressions/beacon request body: ", err)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkBeaconEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	type BeaconImpressions struct {
		Entries json.RawMessage `json:"entries"`
		Sdk     string          `json:"sdk"`
		Token   string          `json:"token"`
	}
	var body BeaconImpressions
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error("Error unmarshaling json in testImpressions/beacon request body: ", err)
		ctx.JSON(http.StatusBadRequest, nil)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkBeaconEndpoint, http.StatusBadRequest)
		return
	}

	if !c.apikeyValidator(&body.Token) {
		c.logger.Error("Unknown/invalid token when parsing testImpressions/beacon request", err)
		ctx.AbortWithStatus(401)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkBeaconEndpoint, http.StatusUnauthorized)
		return
	}

	err = c.impressionsSink.Stage(internal.NewRawImpressions(dtos.Metadata{SDKVersion: "", MachineIP: "NA", MachineName: "NA"}, "", body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Impressions queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push impressions into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusNoContent, nil)
	c.telemetry.IncrEndpointStatus(storage.ImpressionsBulkBeaconEndpoint, http.StatusOK)
}

// TestImpressionsCount accepts impression count payloads
func (c *EventsServerController) TestImpressionsCount(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.ImpressionsCountEndpoint)

	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body in testImpressions/count endpoint: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsCountEndpoint, http.StatusInternalServerError)
		return
	}

	code := http.StatusOK
	err = c.impressionCountSink.Stage(internal.NewRawImpressionCounts(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Impressions count queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push impressions into the staging queue")
		}
		return
	}
	ctx.JSON(code, nil)
	c.telemetry.IncrEndpointStatus(storage.ImpressionsCountEndpoint, code)
}

// TestImpressionsCountBeacon accepts beacon style posts with impression count payload
func (c *EventsServerController) TestImpressionsCountBeacon(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.ImpressionsCountBeaconEndpoint)
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsCountBeaconEndpoint, http.StatusBadRequest)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsCountBeaconEndpoint, http.StatusInternalServerError)
		return
	}

	type BeaconImpressionsCount struct {
		Entries json.RawMessage `json:"entries"`
		Sdk     string          `json:"sdk"`
		Token   string          `json:"token"`
	}
	var body BeaconImpressionsCount
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusBadRequest, nil)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsCountBeaconEndpoint, http.StatusBadRequest)
		return
	}

	if !c.apikeyValidator(&body.Token) {
		ctx.AbortWithStatus(401)
		c.telemetry.IncrEndpointStatus(storage.ImpressionsCountBeaconEndpoint, http.StatusUnauthorized)
		return
	}

	code := http.StatusNoContent

	err = c.impressionCountSink.Stage(internal.NewRawImpressionCounts(dtos.Metadata{SDKVersion: body.Sdk, MachineIP: "NA", MachineName: "NA"}, body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Impressions count queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push impressions into the staging queue")
		}
		return
	}

	ctx.JSON(code, nil)
	c.telemetry.IncrEndpointStatus(storage.ImpressionsCountBeaconEndpoint, code)
}

// EventsBulk accepts incoming event bulks
func (c *EventsServerController) EventsBulk(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.EventsBulkEndpoint)
	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body when accepting an event bulk: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		c.telemetry.IncrEndpointStatus(storage.EventsBulkEndpoint, http.StatusInternalServerError)
		return
	}

	err = c.eventsSink.Stage(internal.NewRawEvents(metadata, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Events queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push events into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusOK, nil)
	c.telemetry.IncrEndpointStatus(storage.EventsBulkEndpoint, http.StatusOK)
}

// EventsBulkBeacon accepts incoming event bulks in a beacon-style request
func (c *EventsServerController) EventsBulkBeacon(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.EventsBulkBeaconEndpoint)
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		c.telemetry.IncrEndpointStatus(storage.EventsBulkBeaconEndpoint, http.StatusBadGateway)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		c.telemetry.IncrEndpointStatus(storage.EventsBulkBeaconEndpoint, http.StatusInternalServerError)
		return
	}

	type BeaconEvents struct {
		Entries json.RawMessage `json:"entries"`
		Sdk     string          `json:"sdk"`
		Token   string          `json:"token"`
	}
	var body BeaconEvents
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusBadRequest, nil)
		c.telemetry.IncrEndpointStatus(storage.EventsBulkBeaconEndpoint, http.StatusBadRequest)
		return
	}

	if !c.apikeyValidator(&body.Token) {
		ctx.AbortWithStatus(401)
		c.telemetry.IncrEndpointStatus(storage.EventsBulkBeaconEndpoint, http.StatusUnauthorized)
		return
	}

	err = c.eventsSink.Stage(internal.NewRawEvents(dtos.Metadata{SDKVersion: "", MachineIP: "NA", MachineName: "NA"}, data))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Events queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push events into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusNoContent, nil)
	c.telemetry.IncrEndpointStatus(storage.EventsBulkBeaconEndpoint, http.StatusNoContent)
}

// DummyAlwaysOk accepts anything and returns 200 without even reading the body
// This is meant to be used with legacy telemetry endpoints
func (c *EventsServerController) DummyAlwaysOk(ctx *gin.Context) {}
