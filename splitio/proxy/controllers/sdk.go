package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	tmw "github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
)

// SdkServerController bundles all request handler for sdk-server apis
type SdkServerController struct {
	logger                  logging.LoggerInterface
	splitBoltDBCollection   *collections.SplitChangesCollection
	segmentBoltDBCollection *collections.SegmentChangesCollection
	telemetry               storage.ProxyEndpointTelemetry
}

// NewSdkServerController instantiates a new sdk server controller
func NewSdkServerController(
	logger logging.LoggerInterface,
	splitBoltDBCollection *collections.SplitChangesCollection,
	segmentBoltDBCollection *collections.SegmentChangesCollection,
	telemetry storage.ProxyEndpointTelemetry,
) *SdkServerController {
	return &SdkServerController{
		logger:                  logger,
		splitBoltDBCollection:   splitBoltDBCollection,
		segmentBoltDBCollection: segmentBoltDBCollection,
		telemetry:               telemetry,
	}
}

// SplitChanges Returns a diff containing changes in splits from a certain point in time until now.
func (c *SdkServerController) SplitChanges(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.SplitChangesEndpoint)
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	sinceParam := ctx.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}
	c.logger.Debug(fmt.Sprintf("SDK Fetches Splits Since: %d", since))

	splits, till, errf := c.fetchSplitsFromDB(since)
	if errf != nil {
		switch errf {
		case boltdb.ErrorBucketNotFound:
			c.logger.Warning("Maybe Splits are not yet synchronized")
		default:
			c.logger.Error(errf)
		}
		c.telemetry.IncrEndpointStatus(storage.SplitChangesEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
		return
	}
	c.telemetry.IncrEndpointStatus(storage.SplitChangesEndpoint, http.StatusOK)
	ctx.JSON(http.StatusOK, gin.H{"splits": splits, "since": since, "till": till})
}

// SegmentChanges Returns a diff containing changes in splits from a certain point in time until now.
func (c *SdkServerController) SegmentChanges(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.SegmentChangesEndpoint)
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	sinceParam := ctx.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}

	segmentName := ctx.Param("name")
	c.logger.Debug(fmt.Sprintf("SDK Fetches Segment: %s Since: %d", segmentName, since))
	added, removed, till, errf := c.fetchSegmentsFromDB(since, segmentName)
	if errf != nil {
		c.telemetry.IncrEndpointStatus(storage.SegmentChangesEndpoint, http.StatusNotFound)
		ctx.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"name":    segmentName,
		"added":   added,
		"removed": removed,
		"since":   since,
		"till":    till,
	})
	c.telemetry.IncrEndpointStatus(storage.SegmentChangesEndpoint, http.StatusOK)
}

// MySegments Returns a diff containing changes in splits from a certain point in time until now.
func (c *SdkServerController) MySegments(ctx *gin.Context) {
	ctx.Set(tmw.EndpointKey, storage.MySegmentsEndpoint)
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	key := ctx.Param("key")
	var mysegments = make([]dtos.MySegmentDTO, 0)

	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB, c.logger)
	segments, errs := segmentCollection.FetchAll()
	if errs != nil {
		c.logger.Warning(errs)
		c.telemetry.IncrEndpointStatus(storage.MySegmentsEndpoint, http.StatusInternalServerError)
		ctx.JSON(http.StatusInternalServerError, gin.H{})
	} else {
		for _, segment := range segments {
			for _, skey := range segment.Keys {
				if !skey.Removed && skey.Name == key {
					mysegments = append(mysegments, dtos.MySegmentDTO{Name: segment.Name})
					break
				}
			}
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"mySegments": mysegments})
	c.telemetry.IncrEndpointStatus(storage.MySegmentsEndpoint, http.StatusOK)
}

func (c *SdkServerController) fetchSplitsFromDB(since int) ([]json.RawMessage, int64, error) {
	till := int64(since)
	splits := make([]json.RawMessage, 0)

	items, err := c.splitBoltDBCollection.FetchAll()
	if err != nil {
		return splits, till, err
	}

	for _, split := range items {
		if split.ChangeNumber > int64(since) {
			if split.ChangeNumber > till {
				till = split.ChangeNumber
			}
			splits = append(splits, []byte(split.JSON))
		}
	}
	return splits, till, nil
}

func (c *SdkServerController) fetchSegmentsFromDB(since int, segmentName string) ([]string, []string, int64, error) {
	added := make([]string, 0)
	removed := make([]string, 0)
	till := int64(since)

	item, err := c.segmentBoltDBCollection.Fetch(segmentName)
	if err != nil {
		switch err {
		case boltdb.ErrorBucketNotFound:
			c.logger.Warning("Bucket not found for segment [%s]\n", segmentName)
		default:
			c.logger.Error(err)
		}
		return added, removed, till, err
	}

	if item == nil {
		return added, removed, till, err
	}

	// Horrible loop borrowed from sdk-api
	for _, skey := range item.Keys {
		if skey.ChangeNumber < int64(since) {
			continue
		}

		// Add the key to the corresponding list
		if skey.Removed && since > 0 {
			removed = append(removed, skey.Name)
		} else {
			added = append(added, skey.Name)
		}

		// Update the till to be returned if necessary
		if since > 0 && skey.ChangeNumber > till {
			till = skey.ChangeNumber
		} else if !skey.Removed && skey.ChangeNumber > till {
			till = skey.ChangeNumber
		}
	}
	return added, removed, till, nil
}
