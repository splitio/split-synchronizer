package controllers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
)

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// FFOverridePayload defines the structure for overriding a feature flag
type FFOverridePayload struct {
	Killed           *bool   `json:"killed"`
	DefaultTreatment *string `json:"defaultTreatment"`
}

// SegmentOverridePayload defines the structure for overriding a segment
type SegmentOverridePayload struct {
	Operation string `json:"operation"` // "Added" or "Removed"
}

// OverrideController bundles endpoints associated to override management
type OverrideController struct {
	logger logging.LoggerInterface
	db     storage.OverrideStorage
}

// NewOverrideController constructs a new override controller
func NewOverrideController(logger logging.LoggerInterface, db storage.OverrideStorage) *OverrideController {
	return &OverrideController{logger: logger, db: db}
}

// Register mounts the endpoints in the provided router
func (c *OverrideController) Register(router gin.IRouter) {
	router.GET("/overrides/ff", c.getOverridesFeatureFlags)
	router.POST("/overrides/ff/:name", c.overrideFeatureFlag)
	router.DELETE("/overrides/ff/:name", c.deleteFeatureFlag)

	router.GET("/overrides/segment", c.getOverridesForSegments)
	router.POST("/overrides/segment/:name/:key", c.overrideSegment)
	router.DELETE("/overrides/segment/:name/:key", c.deleteSegmentOverride)
}

// @Summary retrieves all feature flag overrides
// @Description retrieves all feature flag overrides
// @Tags override
// @Accept json
// @Produce json
// @Success 200 {object} []SplitDTO
// @Failure 500 {object} APIError
func (c *OverrideController) getOverridesFeatureFlags(ctx *gin.Context) {
	overrides := c.db.GetOverrides()
	if overrides == nil {
		c.logger.Error("overrides.ff: no feature flag overrides found")
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, APIError{Code: http.StatusInternalServerError, Message: "No feature flag overrides found"})
		return
	}

	ctx.JSON(http.StatusOK, overrides)
}

// @Summary overrides a feature flag
// @Description overrides a feature flag with a specific name
// @Tags override
// @Accept json
// @Produce json
// @Param name path string true "Feature flag name"
// @Param request body FFOverridePayload true "the request body"
// @Success 200 {object} SplitDTO
// @Failure 400 {object} APIError
// @Failure 404 {object} APIError
// @Failure 500 {object} APIError
// @Router overrides/ff/{name} [post]
func (c *OverrideController) overrideFeatureFlag(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		c.logger.Error("overrides.ff: feature flag name is required")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Feature flag name is required"})
		return
	}

	var overridePayload FFOverridePayload
	if err := ctx.ShouldBindJSON(&overridePayload); err != nil {
		c.logger.Error(fmt.Sprintf("overrides.ff: error parsing payload: %s", err.Error()))
		ctx.AbortWithStatusJSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	ff, err := c.db.OverrideFF(name, overridePayload.Killed, overridePayload.DefaultTreatment)
	if err != nil {
		c.logger.Error(fmt.Sprintf("overrides.ff: error saving override for feature flag %s: %s", name, err.Error()))
		if errors.Is(err, storage.ErrFeatureFlagNotFound) {
			ctx.AbortWithStatusJSON(http.StatusNotFound, APIError{Code: http.StatusNotFound, Message: err.Error()})
			return
		}
		ctx.AbortWithStatusJSON(http.StatusBadRequest, APIError{Code: http.StatusInternalServerError, Message: err.Error()})
		return
	}

	ctx.JSON(200, ff)
}

// @Summary deletes a feature flag override
// @Description deletes a feature flag override with a specific name
// @Tags override
// @Accept json
// @Produce json
// @Param name path string true "Feature flag name"
// @Success 200 {object} SplitDTO
// @Failure 500 {object} APIError
// @Router overrides/ff/{name} [delete]
func (c *OverrideController) deleteFeatureFlag(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		c.logger.Error("overrides.ff: feature flag name is required")
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Feature flag name is required"})
		return
	}

	c.db.RemoveOverrideFF(name)
	ctx.JSON(200, nil)
}

// @Summary retrieves all segment overrides
// @Description retrieves all segment overrides
// @Tags override
// @Accept json
// @Produce json
// @Success 200 {object} map[string][]PerKey
// @Failure 500 {object} APIError
func (c *OverrideController) getOverridesForSegments(ctx *gin.Context) {
	overrides := c.db.GetOverridesForSegment()
	if overrides == nil {
		c.logger.Error("overrides.segment: no segment overrides found")
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, APIError{Code: http.StatusInternalServerError, Message: "No segment overrides found"})
		return
	}

	ctx.JSON(http.StatusOK, overrides)
}

// @Summary overrides a segment for a specific user key
// @Description overrides a segment for a specific user key with the specified operation
// @Tags override
// @Accept json
// @Produce json
// @Param name path string true "Segment name"
// @Param key path string true "User Key"
// @Param request body SegmentOverridePayload true "the request body"
// @Success 200 {object} SegmentOverride
// @Failure 400 {object} APIError
// @Failure 500 {object} APIError
// @Router overrides/segment/{name}/{key} [post]
func (c *OverrideController) overrideSegment(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		c.logger.Error("overrides.overrideSegment: segment name is required")
		ctx.JSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: "segment name is required"})
		return
	}
	key := ctx.Param("key")
	if key == "" {
		c.logger.Error("overrides.overrideSegment: user key is required")
		ctx.JSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: "user key is required"})
		return
	}

	var overridePayload SegmentOverridePayload
	if err := ctx.ShouldBindJSON(&overridePayload); err != nil {
		c.logger.Error(fmt.Sprintf("overrides.overrideSegment: error parsing payload: %s", err.Error()))
		ctx.AbortWithStatusJSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}

	if overridePayload.Operation != "add" && overridePayload.Operation != "remove" {
		c.logger.Error(fmt.Sprintf("overrides.overrideSegment: invalid operation: %s", overridePayload.Operation))
		ctx.AbortWithStatusJSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: "invalid operation"})
		return
	}

	ctx.JSON(200, c.db.OverrideSegment(key, name, overridePayload.Operation))
}

// @Summary deletes a segment override for a specific user key
// @Description deletes a segment override for a specific user key
// @Tags override
// @Accept json
// @Produce json
// @Param key path string true "User Key"
// @Success 200 {object} SegmentOverride
// @Failure 500 {object} APIError
// @Router overrides/segment/{name}/{key} [delete]
func (c *OverrideController) deleteSegmentOverride(ctx *gin.Context) {
	key := ctx.Param("key")
	if key == "" {
		c.logger.Error("overrides.deleteSegment: user key is required")
		ctx.JSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: "user key is required"})
		return
	}

	name := ctx.Param("name")
	if name == "" {
		c.logger.Error("overrides.deleteSegment: segment name is required")
		ctx.JSON(http.StatusBadRequest, APIError{Code: http.StatusBadRequest, Message: "segment name is required"})
		return
	}

	c.db.RemoveOverrideSegment(key, name)
	ctx.JSON(200, nil)
}

/*
curl -XPOST 'http://localhost:3010/admin/overrides/ff/TEST_MATIAS' \
--header 'Content-Type: application/json' \
--data '{
  "killed": true,
  "defaultTreatment": "on"
}'

curl -XPOST 'http://localhost:3010/admin/overrides/ff/MATIAS_TEST' \
--header 'Content-Type: application/json' \
--data '{
  "killed": true,
  "defaultTreatment": "on"
}'

curl -XGET 'http://localhost:3010/admin/overrides/ff'

curl -XDELETE 'http://localhost:3010/admin/overrides/ff/TEST_MATIAS'

curl -XDELETE 'http://localhost:3010/admin/overrides/ff/MATIAS_TEST'

curl -XPOST 'http://localhost:3010/admin/overrides/segment/segment1/key1' \
--header 'Content-Type: application/json' \
--data '{
  "operation": "add"
}'

curl -XPOST 'http://localhost:3010/admin/overrides/segment/segment2/key1' \
--header 'Content-Type: application/json' \
--data '{
  "operation": "remove"
}'

curl -XDELETE 'http://localhost:3010/admin/overrides/segment/segment2/key1'

curl -XGET 'http://localhost:3010/admin/overrides/segment'
*/
