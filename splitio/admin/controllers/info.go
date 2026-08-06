package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio"
	"github.com/splitio/split-synchronizer/v5/splitio/common"

	"github.com/gin-gonic/gin"
)

// InfoController contains handlers for system information purposes
type InfoController struct {
	proxy   bool
	runtime common.Runtime
	cfg     interface{}
}

// NewInfoController constructs a new InfoController to be mounted on a gin router
func NewInfoController(proxy bool, runtime common.Runtime, config interface{}) *InfoController {
	return &InfoController{
		proxy:   proxy,
		runtime: runtime,
		cfg:     config,
	}
}

// Register info controller endpoints
func (c *InfoController) Register(router gin.IRouter) {
	router.GET("/uptime", c.uptime)
	router.GET("/version", c.version)
	router.GET("/ping", c.ping)
	router.GET("/config", c.config)
}

func (c *InfoController) config(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"config": c.cfg})
}

func (c *InfoController) uptime(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"uptime": fmt.Sprintf("%s", c.runtime.Uptime().Round(time.Second))})
}

func (c *InfoController) version(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"version": splitio.Version})
}

func (c *InfoController) ping(ctx *gin.Context) {
	ctx.String(http.StatusOK, "%s", "pong")
}
