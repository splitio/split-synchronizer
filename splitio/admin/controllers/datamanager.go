package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

var errMissingSize = errors.New("missing 'size' query param")

// ImpressionFlusher defines methods required for flushing impressions
type ImpressionFlusher interface{ FlushImpressions(count int64) error }

// EventFlusher defines methods requried for flushing events
type EventFlusher interface{ FlushEvents(count int64) error }

// DataManagerController groups endpoints related to flushing & dropping impressions & events
type DataManagerController struct {
	impressionManipulator storage.DataDropper
	eventManipulator      storage.DataDropper
	impressionRecorder    ImpressionFlusher
	eventRecorder         EventFlusher
	logger                logging.LoggerInterface
	basepath              string
}

// NewDataManagerController provides a group of endpoints for flushing & dropping accumulated user-generated data
func NewDataManagerController(
	impressionManipulator storage.DataDropper,
	eventManipulator storage.DataDropper,
	impressionRecorder ImpressionFlusher,
	eventRecorder EventFlusher,
	logger logging.LoggerInterface,
	basepath string,
) *DataManagerController {
	return &DataManagerController{
		impressionManipulator: impressionManipulator,
		eventManipulator:      eventManipulator,
		impressionRecorder:    impressionRecorder,
		eventRecorder:         eventRecorder,
		logger:                logger,
		basepath:              basepath,
	}
}

// Register mounts the controller endpoint on top of an IRouter interface.
func (c *DataManagerController) Register(router gin.IRouter) {
	router.POST("/impressions/flush", c.FlushImpressions)
	router.POST("/impressions/drop", c.DropImpressions)
	router.POST("/events/flush", c.FlushEvents)
	router.POST("/events/drop", c.DropEvents)
}

// BasePath returns the path where the controller is mounted
func (c *DataManagerController) BasePath() string {
	return c.basepath
}

// DropImpressions drops impressions
func (c *DataManagerController) DropImpressions(ctx *gin.Context) {
	size, err := getSize(ctx)
	if err != nil {
		c.logger.Error("error parsing size: ", err)
		ctx.String(http.StatusBadRequest, fmt.Sprintf("error parsing size: %s", err.Error()))
		return
	}

	err = c.impressionManipulator.Drop(size)
	if err == nil {
		ctx.String(http.StatusOK, "Impressions dropped")
		return
	}
	ctx.String(http.StatusInternalServerError, "%s", err.Error())
}

// DropEvents drops events
func (c *DataManagerController) DropEvents(ctx *gin.Context) {
	size, err := getSize(ctx)
	if err != nil {
		c.logger.Error("error parsing size: ", err)
		ctx.String(http.StatusBadRequest, fmt.Sprintf("error parsing size: %s", err.Error()))
		return
	}

	err = c.eventManipulator.Drop(size)
	if err == nil {
		ctx.String(http.StatusOK, "Events dropped")
		return
	}
	ctx.String(http.StatusInternalServerError, "%s", err.Error())
}

// FlushImpressions flushes impressions
func (c *DataManagerController) FlushImpressions(ctx *gin.Context) {
	size, err := getSize(ctx)
	if err != nil {
		c.logger.Error("error parsing size: ", err)
		ctx.String(http.StatusBadRequest, fmt.Sprintf("error parsing size: %s", err.Error()))
		return
	}

	err = c.impressionRecorder.FlushImpressions(size)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	ctx.String(http.StatusOK, "Impressions flushed")
}

// FlushEvents flushes events
func (c *DataManagerController) FlushEvents(ctx *gin.Context) {
	size, err := getSize(ctx)
	if err != nil {
		c.logger.Error("error parsing size: ", err)
		ctx.String(http.StatusBadRequest, fmt.Sprintf("error parsing size: %s", err.Error()))
		return
	}

	err = c.eventRecorder.FlushEvents(size)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	ctx.String(http.StatusOK, "Events flushed")
}

func getSize(ctx *gin.Context) (int64, error) {
	size, ok := ctx.GetQuery("size")
	if !ok {
		// If no size was passed we flush everything
		return 0, nil
	}

	asInt, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse size '%s' as integer: %w", size, err)
	}

	return asInt, nil
}
