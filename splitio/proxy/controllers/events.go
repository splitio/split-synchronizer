package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/internal"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"
)

// EventsServerController bundles all request handler for sdk-server apis
type EventsServerController struct {
	logger              logging.LoggerInterface
	impressionsSink     tasks.DeferredRecordingTask
	impressionCountSink tasks.DeferredRecordingTask
	eventsSink          tasks.DeferredRecordingTask
	listener            impressionlistener.ImpressionBulkListener
	apikeyValidator     func(string) bool
}

// NewEventsServerController returns a new events server controller
func NewEventsServerController(
	logger logging.LoggerInterface,
	impressionsSink tasks.DeferredRecordingTask,
	impressionCountSink tasks.DeferredRecordingTask,
	eventsSink tasks.DeferredRecordingTask,
	listener impressionlistener.ImpressionBulkListener,
	apikeyValidator func(string) bool,
) *EventsServerController {
	return &EventsServerController{
		logger:              logger,
		impressionsSink:     impressionsSink,
		impressionCountSink: impressionCountSink,
		eventsSink:          eventsSink,
		listener:            listener,
		apikeyValidator:     apikeyValidator,
	}
}

// Register mounts events-server endpoints onto the suppplied routers
func (c *EventsServerController) Register(regular gin.IRouter, beacon gin.IRouter) {
	regular.POST("/testImpressions/bulk", c.TestImpressionsBulk)
	regular.POST("/testImpressions/count", c.TestImpressionsCount)
	regular.POST("/events/bulk", c.EventsBulk)

	// dummy endpoints that just return 200
	regular.POST("/metrics/times", c.DummyAlwaysOk)
	regular.POST("/metrics/counters", c.DummyAlwaysOk)
	regular.POST("/metrics/gauge", c.DummyAlwaysOk)
	regular.POST("/metrics/time", c.DummyAlwaysOk)
	regular.POST("/metrics/counter", c.DummyAlwaysOk)

	// beacon endpoints
	beacon.POST("/testImpressions/count/beacon", c.TestImpressionsCountBeacon)
	beacon.POST("/testImpressions/beacon", c.TestImpressionsBeacon)
	beacon.POST("/events/beacon", c.EventsBulkBeacon)
}

// TestImpressionsBulk endpoint accepts impression bulks
func (c *EventsServerController) TestImpressionsBulk(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	impressionsMode := parseImpressionsMode(ctx.Request.Header.Get("SplitSDKImpressionsMode"))
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error(err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}
	if c.listener != nil {
		// if we have a listener, schedule a goroutine to convert these impressions and
		// push them into the channel.
		go c.submitImpressionsToListener(data, &metadata)
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
}

// TestImpressionsBeacon accepts beacon style posts with impressions payload
func (c *EventsServerController) TestImpressionsBeacon(ctx *gin.Context) {
	if ctx.Request.Body == nil {
		c.logger.Error("Nil body when testImpressions/beacon request.")

		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading testImpressions/beacon request body: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	var body beaconMessage
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		c.logger.Error("Error unmarshaling json in testImpressions/beacon request body: ", err)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	if !c.apikeyValidator(body.Token) {
		c.logger.Error("Unknown/invalid token when parsing testImpressions/beacon request", err)
		ctx.AbortWithStatus(401)
		return
	}

	err = c.impressionsSink.Stage(internal.NewRawImpressions(dtos.Metadata{SDKVersion: body.Sdk, MachineIP: "NA", MachineName: "NA"}, "", body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Impressions queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push impressions into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusNoContent, nil)
}

// TestImpressionsCount accepts impression count payloads
func (c *EventsServerController) TestImpressionsCount(ctx *gin.Context) {

	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body in testImpressions/count endpoint: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
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
}

// TestImpressionsCountBeacon accepts beacon style posts with impression count payload
func (c *EventsServerController) TestImpressionsCountBeacon(ctx *gin.Context) {
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
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
}

// EventsBulk accepts incoming event bulks
func (c *EventsServerController) EventsBulk(ctx *gin.Context) {
	metadata := metadataFromHeaders(ctx)
	data, err := ioutil.ReadAll(ctx.Request.Body)
	if err != nil {
		c.logger.Error("Error reading request body when accepting an event bulk: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
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
}

// EventsBulkBeacon accepts incoming event bulks in a beacon-style request
func (c *EventsServerController) EventsBulkBeacon(ctx *gin.Context) {
	if ctx.Request.Body == nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	data, err := ioutil.ReadAll(ctx.Request.Body)
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

	err = c.eventsSink.Stage(internal.NewRawEvents(dtos.Metadata{SDKVersion: body.Sdk, MachineIP: "NA", MachineName: "NA"}, body.Entries))
	if err != nil {
		if err == tasks.ErrQueueFull {
			ctx.AbortWithStatusJSON(500, "Events queue is full, please retry later.")
		} else {
			ctx.AbortWithStatusJSON(500, "Unknown error when trying to push events into the staging queue")
		}
		return
	}
	ctx.JSON(http.StatusNoContent, nil)
}

// DummyAlwaysOk accepts anything and returns 200 without even reading the body
// This is meant to be used with legacy telemetry endpoints
func (c *EventsServerController) DummyAlwaysOk(ctx *gin.Context) {}

func (c *EventsServerController) submitImpressionsToListener(raw []byte, metadata *dtos.Metadata) {
	var parsed []dtos.ImpressionsDTO
	err := json.Unmarshal(raw, &parsed)
	if err != nil {
		c.logger.Error("error when parsing impressions prior to being forwarded to the listener: ", err)
		return
	}

	forListener := make([]impressionlistener.ImpressionsForListener, 0, len(parsed))
	for _, group := range parsed {
		kis := make([]impressionlistener.ImpressionForListener, 0, len(group.KeyImpressions))
		for _, ki := range group.KeyImpressions {
			kis = append(kis, impressionlistener.ImpressionForListener{
				KeyName:      ki.KeyName,
				Treatment:    ki.Treatment,
				Time:         ki.Time,
				ChangeNumber: ki.ChangeNumber,
				Label:        ki.Label,
				BucketingKey: ki.BucketingKey,
				Pt:           ki.Pt,
			})
		}
		forListener = append(forListener, impressionlistener.ImpressionsForListener{
			TestName:       group.TestName,
			KeyImpressions: kis,
		})
	}

	c.listener.Submit(forListener, metadata)
}

// private dtos
type beaconMessage struct {
	Entries json.RawMessage `json:"entries"`
	Sdk     string          `json:"sdk"`
	Token   string          `json:"token"`
}
