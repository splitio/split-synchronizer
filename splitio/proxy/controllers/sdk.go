package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/flagsets"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-split-commons/v9/engine/validator"
	"github.com/splitio/go-split-commons/v9/service"
	"github.com/splitio/go-split-commons/v9/service/api/specs"
	cmnStorage "github.com/splitio/go-split-commons/v9/storage"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slices"
)

// SdkServerController bundles all request handler for sdk-server apis
type SdkServerController struct {
	logger                logging.LoggerInterface
	fetcher               service.SplitFetcher
	proxySplitStorage     storage.ProxySplitStorage
	proxyRBSegmentStorage storage.ProxyRuleBasedSegmentsStorage
	proxySegmentStorage   storage.ProxySegmentStorage
	fsmatcher             flagsets.FlagSetMatcher
	versionFilter         specs.SplitVersionFilter
	largeSegmentStorage   cmnStorage.LargeSegmentsStorage
	specVersion           string
}

// NewSdkServerController instantiates a new sdk server controller
func NewSdkServerController(
	logger logging.LoggerInterface,
	fetcher service.SplitFetcher,
	proxySplitStorage storage.ProxySplitStorage,
	proxySegmentStorage storage.ProxySegmentStorage,
	proxyRBSegmentStorage storage.ProxyRuleBasedSegmentsStorage,
	fsmatcher flagsets.FlagSetMatcher,
	largeSegmentStorage cmnStorage.LargeSegmentsStorage,
	specVersion string,
) *SdkServerController {
	return &SdkServerController{
		logger:                logger,
		fetcher:               fetcher,
		proxySplitStorage:     proxySplitStorage,
		proxySegmentStorage:   proxySegmentStorage,
		proxyRBSegmentStorage: proxyRBSegmentStorage,
		fsmatcher:             fsmatcher,
		versionFilter:         specs.NewSplitVersionFilter(),
		largeSegmentStorage:   largeSegmentStorage,
		specVersion:           specVersion,
	}
}

// Register mounts the sdk-server endpoints onto the supplied router
func (c *SdkServerController) Register(router gin.IRouter) {
	router.GET("/splitChanges", c.SplitChanges)
	router.GET("/segmentChanges/:name", c.SegmentChanges)
	router.GET("/mySegments/:key", c.MySegments)
	router.GET("/memberships/:key", c.Memberships)
}

func (c *SdkServerController) Memberships(ctx *gin.Context) {
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	key := ctx.Param("key")
	segmentList, err := c.proxySegmentStorage.SegmentsFor(key)
	if err != nil {
		c.logger.Error(fmt.Sprintf("error fetching segments for user '%s': %s", key, err.Error()))
		ctx.JSON(http.StatusInternalServerError, gin.H{})
		return
	}

	mySegments := make([]dtos.Segment, 0, len(segmentList))
	for _, segmentName := range segmentList {
		mySegments = append(mySegments, dtos.Segment{Name: segmentName})
	}

	lsList := c.largeSegmentStorage.LargeSegmentsForUser(key)
	myLargeSegments := make([]dtos.Segment, 0, len(lsList))
	for _, name := range lsList {
		myLargeSegments = append(myLargeSegments, dtos.Segment{Name: name})
	}

	payoad := dtos.MembershipsResponseDTO{
		MySegments: dtos.Memberships{
			Segments: mySegments,
		},
		MyLargeSegments: dtos.Memberships{
			Segments: myLargeSegments,
		},
	}

	ctx.JSON(http.StatusOK, payoad)
	ctx.Set(caching.SurrogateContextKey, []string{caching.MembershipsSurrogate})
}

// SplitChanges Returns a diff containing changes in feature flags from a certain point in time until now.
func (c *SdkServerController) SplitChanges(ctx *gin.Context) {
	c.logger.Debug(fmt.Sprintf("Headers: %v", ctx.Request.Header))
	since, err := strconv.ParseInt(ctx.DefaultQuery("since", "-1"), 10, 64)
	if err != nil {
		since = -1
	}

	rbsince, err := strconv.ParseInt(ctx.DefaultQuery("rbSince", "-1"), 10, 64)
	if err != nil {
		rbsince = -1
	}

	var rawSets []string
	if fq, ok := ctx.GetQuery("sets"); ok {
		rawSets = strings.Split(fq, ",")
	}
	sets := c.fsmatcher.Sanitize(rawSets)
	if !slices.Equal(sets, rawSets) {
		c.logger.Warning(fmt.Sprintf("SDK [%s] is sending flagsets unordered or with duplicates.", ctx.Request.Header.Get("SplitSDKVersion")))
	}

	c.logger.Debug(fmt.Sprintf("SDK Fetches Feature Flags Since: %d, RBSince: %d", since, rbsince))

	rules, err := c.fetchRulesSince(since, rbsince, sets)
	if err != nil {
		c.logger.Error("error fetching splitChanges payload from storage: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sParam, _ := ctx.GetQuery("s")
	spec, err := specs.ParseAndValidate(sParam)
	if err != nil {
		c.logger.Error(fmt.Sprintf("error parsing spec version: %s.", err))
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	rules.FeatureFlags.Splits = c.patchUnsupportedMatchers(rules.FeatureFlags.Splits, spec)

	if spec == specs.FLAG_V1_3 {
		ctx.JSON(http.StatusOK, rules)
		ctx.Set(caching.SurrogateContextKey, []string{caching.SplitSurrogate})
		ctx.Set(caching.StickyContextKey, true)
		return
	}
	ctx.JSON(http.StatusOK, dtos.SplitChangesDTO{
		Splits: rules.FeatureFlags.Splits,
		Since:  rules.FeatureFlags.Since,
		Till:   rules.FeatureFlags.Till,
	})
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

func (c *SdkServerController) fetchRulesSince(since int64, rbsince int64, sets []string) (*dtos.RuleChangesDTO, error) {
	splits, err := c.proxySplitStorage.ChangesSince(since, sets)
	rbs, rbsErr := c.proxyRBSegmentStorage.ChangesSince(rbsince)
	if err == nil && rbsErr == nil {
		return &dtos.RuleChangesDTO{
			FeatureFlags: dtos.FeatureFlagsDTO{
				Splits: splits.Splits,
				Till:   splits.Till,
				Since:  splits.Since,
			},
			RuleBasedSegments: *rbs,
		}, err
	}
	if err != nil && !errors.Is(err, storage.ErrSinceParamTooOld) {
		return nil, fmt.Errorf("unexpected error fetching feature flag changes from storage: %w", err)
	}

	if rbsErr != nil && !errors.Is(rbsErr, storage.ErrSinceParamTooOld) {
		return nil, fmt.Errorf("unexpected error fetching rule-based segments changes from storage: %w", rbsErr)
	}

	// perform a fetch to the BE using the supplied `since`, have the storage process it's response &, retry
	// TODO(mredolatti): implement basic collapsing here to avoid flooding the BE with requests
	fetchOptions := service.MakeFlagRequestParams().WithSpecVersion(common.StringRef(c.specVersion)).WithChangeNumber(since).WithChangeNumberRB(rbsince).WithFlagSetsFilter(strings.Join(sets, ",")) // at this point the sets have been sanitized & sorted
	ruleChanges, err := c.fetcher.Fetch(fetchOptions)
	if err != nil {
		return nil, err
	}
	return &dtos.RuleChangesDTO{
		FeatureFlags: dtos.FeatureFlagsDTO{
			Splits: ruleChanges.FeatureFlags(),
			Till:   ruleChanges.FFTill(),
			Since:  ruleChanges.FFSince(),
		},
		RuleBasedSegments: dtos.RuleBasedSegmentsDTO{
			RuleBasedSegments: ruleChanges.RuleBasedSegments(),
			Till:              ruleChanges.RBTill(),
			Since:             ruleChanges.RBSince(),
		},
	}, nil
}

func (c *SdkServerController) shouldOverrideSplitCondition(split *dtos.SplitDTO, version string) bool {
	for _, condition := range split.Conditions {
		for _, matcher := range condition.MatcherGroup.Matchers {
			if c.versionFilter.ShouldFilter(matcher.MatcherType, version) {
				return true
			}
		}
	}
	return false
}

func (c *SdkServerController) patchUnsupportedMatchers(splits []dtos.SplitDTO, version string) []dtos.SplitDTO {
	for si := range splits {
		if c.shouldOverrideSplitCondition(&splits[si], version) {
			splits[si].Conditions = validator.MakeUnsupportedMatcherConditionReplacement()
		}
	}
	return splits
}
