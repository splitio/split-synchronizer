package proxy

import (
	"gopkg.in/gin-gonic/gin.v1"
)

func Run(port string) {
	//gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	//router := gin.New()
	//TODO add custom logger as middleware (?)
	//router.Use(gin.Recovery())

	router.GET("/api/splitChanges", splitChanges)
	router.GET("/api/segmentChanges/:name", segmentChanges)

	//impressions
	router.POST("/api/testImpressions/bulk", postBulkImpressions)

	//metrics
	//latencies
	router.POST("/api/metrics/times", postMetricsTimes)

	//counters
	router.POST("/api/metrics/counters", postMetricsCounters)

	//gauge
	router.POST("/api/metrics/gauge", postMetricsGauge)

	router.Run(port)
}
