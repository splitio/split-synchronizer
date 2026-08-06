package controllers

import (
	"github.com/splitio/go-split-commons/v9/conf"
	"github.com/splitio/go-split-commons/v9/dtos"

	"github.com/gin-gonic/gin"
)

func metadataFromHeaders(ctx *gin.Context) dtos.Metadata {
	return dtos.Metadata{
		SDKVersion:  ctx.Request.Header.Get("SplitSDKVersion"),
		MachineIP:   ctx.Request.Header.Get("SplitSDKMachineIP"),
		MachineName: ctx.Request.Header.Get("SplitSDKMachineName"),
	}
}

func parseImpressionsMode(mode string) string {
	if mode == conf.ImpressionsModeOptimized {
		return mode
	}
	return conf.ImpressionsModeDebug
}
