package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v6/conf"
	"github.com/splitio/go-split-commons/v6/dtos"
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
