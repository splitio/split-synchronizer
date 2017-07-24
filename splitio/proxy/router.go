package proxy

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func Run(port string, adminPort string) {
	//gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	//TODO add custom logger as middleware (?)
	router.Use(gin.Logger())

	go func() {
		adminRouter := gin.Default()
		// Admin routes
		admin := adminRouter.Group("/admin")
		{
			admin.GET("/ping", ping)
			admin.GET("/version", version)
			admin.GET("/uptime", uptime)
			admin.GET("/stats", showStats)
			admin.GET("/dashboard", showDashboard)
		}

		adminRouter.Run(adminPort)
	}()

	// API routes
	api := router.Group("/api")
	{
		api.GET("/splitChanges", splitChanges)
		api.GET("/segmentChanges/:name", segmentChanges)
		api.POST("/testImpressions/bulk", postBulkImpressions)
		api.POST("/metrics/times", postMetricsTimes)
		api.POST("/metrics/counters", postMetricsCounters)
		api.POST("/metrics/gauge", postMetricsGauge)

		//TODO add single metrics endpoints
		//api.POST("/metrics/time", postMetricsTimes)
		//api.POST("/metrics/counter", postMetricsCounters)
	}
	router.Run(port)
}
