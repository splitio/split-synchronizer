package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/splitio/go-split-commons/v5/service"
	"github.com/splitio/go-toolkit/v5/logging"
	"golang.org/x/exp/slices"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
)

// SdkServerController bundles all request handler for sdk-server apis
type SdkServerController struct {
	logger              logging.LoggerInterface
	fetcher             service.SplitFetcher
	proxySplitStorage   storage.ProxySplitStorage
	proxySegmentStorage storage.ProxySegmentStorage
	fsmatcher           flagsets.FlagSetMatcher
}

// NewSdkServerController instantiates a new sdk server controller
func NewSdkServerController(
	logger logging.LoggerInterface,
	fetcher service.SplitFetcher,
	proxySplitStorage storage.ProxySplitStorage,
	proxySegmentStorage storage.ProxySegmentStorage,
	fsmatcher flagsets.FlagSetMatcher,

) *SdkServerController {
	return &SdkServerController{
		logger:              logger,
		fetcher:             fetcher,
		proxySplitStorage:   proxySplitStorage,
		proxySegmentStorage: proxySegmentStorage,
		fsmatcher:           fsmatcher,
	}
}

// Register mounts the sdk-server endpoints onto the supplied router
func (c *SdkServerController) Register(router gin.IRouter) {
	router.GET("/splitChanges", c.SplitChanges)
	router.GET("/segmentChanges/:name", c.SegmentChanges)
	router.GET("/mySegments/:key", c.MySegments)
}

// SplitChanges Returns a diff containing changes in feature flags from a certain point in time until now.
func (c *SdkServerController) SplitChanges(ctx *gin.Context) {
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	since, err := strconv.ParseInt(ctx.DefaultQuery("since", "-1"), 10, 64)
	if err != nil {
		since = -1
	}

	var rawSets []string
	if fq, ok := ctx.GetQuery("sets"); ok {
		rawSets = strings.Split(fq, ",")
	}
	sets := c.fsmatcher.Sanitize(rawSets)
	if !slices.Equal(sets, rawSets) {
		c.logger.Warning(fmt.Sprintf("SDK [%s] is sending flagsets unordered or with duplicates.", ctx.Request.Header.Get("SplitSDKVersion")))
	}

	c.logger.Debug(fmt.Sprintf("SDK Fetches Feature Flags Since: %d", since))

	splits, err := c.fetchSplitChangesSince(since, sets)
	if err != nil {
		c.logger.Error("error fetching splitChanges payload from storage: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, splits)
	ctx.Set(caching.SurrogateContextKey, []string{caching.SplitSurrogate})
	ctx.Set(caching.StickyContextKey, true)
}

// SegmentChanges Returns a diff containing changes in feature flags from a certain point in time until now.
func (c *SdkServerController) SegmentChanges(ctx *gin.Context) {
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	since, err := strconv.ParseInt(ctx.DefaultQuery("since", "-1"), 10, 64)
	if err != nil {
		since = -1
	}

	segmentName := ctx.Param("name")
	c.logger.Debug(fmt.Sprintf("SDK Fetches Segment: %s Since: %d", segmentName, since))
	payload, err := c.proxySegmentStorage.ChangesSince(segmentName, since)
	if err != nil {
		if errors.Is(err, storage.ErrSegmentNotFound) {
			c.logger.Error("the following segment was requested and is not present: ", segmentName)
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.logger.Error("error fetching segmentChanges payload from storage: ", err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	ctx.JSON(http.StatusOK, payload)
	ctx.Set(caching.SurrogateContextKey, []string{caching.MakeSurrogateForSegmentChanges(segmentName)})
	ctx.Set(caching.StickyContextKey, true)
}

// MySegments Returns a diff containing changes in feature flags from a certain point in time until now.
func (c *SdkServerController) MySegments(ctx *gin.Context) {
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	key := ctx.Param("key")
	segmentList, err := c.proxySegmentStorage.SegmentsFor(key)
	if err != nil {
		c.logger.Error(fmt.Sprintf("error fetching segments for user '%s': %s", key, err.Error()))
		ctx.JSON(http.StatusInternalServerError, gin.H{})
	}

	mySegments := make([]dtos.MySegmentDTO, 0, len(segmentList))
	for _, segmentName := range segmentList {
		mySegments = append(mySegments, dtos.MySegmentDTO{Name: segmentName})
	}

	ctx.JSON(http.StatusOK, gin.H{"mySegments": mySegments})
	ctx.Set(caching.SurrogateContextKey, caching.MakeSurrogateForMySegments(mySegments))
}

func (c *SdkServerController) fetchSplitChangesSince(since int64, sets []string) (*dtos.SplitChangesDTO, error) {
	splits, err := c.proxySplitStorage.ChangesSince(since, sets)
	if err == nil {
		return splits, nil
	}
	if !errors.Is(err, storage.ErrSummaryNotCached) {
		return nil, fmt.Errorf("unexpected error fetching feature flag changes from storage: %w", err)
	}

	fetchOptions := service.NewFetchOptions(true, nil)
	splits, err = c.fetcher.Fetch(since, &fetchOptions)
	if err == nil {
		c.proxySplitStorage.RegisterOlderCn(splits)
		return splits, nil
	}
	return nil, fmt.Errorf("unexpected error fetching feature flag changes from storage: %w", err)
}
