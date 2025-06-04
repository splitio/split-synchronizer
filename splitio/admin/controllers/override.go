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

type FFOverridePayload struct {
	Killed           *bool   `json:"killed"`
	DefaultTreatment *string `json:"defaultTreatment"`
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
	router.POST("/overrides/ff/:name", c.overrideFeatureFlag)
	router.DELETE("/overrides/ff/:name", c.deleteFeatureFlag)
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
		c.logger.Error(fmt.Sprintf("overrides.ff: feature flag name is required: %s", err.Error()))
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

/*
curl -XPOST 'http://localhost:3010/admin/overrides/ff/TEST_MATIAS' \
--header 'Content-Type: application/json' \
--data '{
  "killed": true,
  "defaultTreatment": "on"
}'

curl -XDELETE 'http://localhost:3010/admin/overrides/ff/TEST_MATIAS'
*/
