package proxy

import (
	"net/http"

	"gopkg.in/gin-gonic/gin.v1"
)

func Run(port string) {
	//gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	//TODO add custom logger as middleware (?)
	router.Use(gin.Logger())

	/*go func() {
		adminRouter := gin.Default()
		adminRouter.GET("/ping", func(c *gin.Context) {
			c.String(http.StatusOK, "%s", "pong")
		})
		adminRouter.Run("0.0.0.0:3010")
	}()*/

	//Admin route
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "%s", "pong")
	})

	// API routes
	api := router.Group("/api")
	{
		api.GET("/splitChanges", splitChanges)
		api.GET("/segmentChanges/:name", segmentChanges)
		api.POST("/testImpressions/bulk", postBulkImpressions)
		api.POST("/metrics/times", postMetricsTimes)
		api.POST("/metrics/counters", postMetricsCounters)
		api.POST("/metrics/gauge", postMetricsGauge)
	}
	router.Run(port)
}
