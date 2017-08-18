package proxy

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/splitio/proxy/middleware"
)

// ProxyOptions struct to set options for Proxy mode.
type ProxyOptions struct {
	Port                      string
	AdminPort                 string
	AdminUsername             string
	AdminPassword             string
	APIKeys                   []string
	ImpressionListenerEnabled bool
	DebugOn                   bool
}

// Run runs the proxy server
func Run(options *ProxyOptions) {
	if !options.DebugOn {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	//CORS - Allows all origins
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"SplitSDKMachineName",
		"SplitSDKMachineIP",
		"SplitSDKVersion",
		"Authorization"}
	router.Use(cors.New(corsConfig))

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.Logger())
	router.Use(middleware.ValidateAPIKeys(options.APIKeys))

	// running admin endpoints
	go func() {
		adminRouter := gin.New()
		adminRouter.Use(gin.Recovery())
		adminRouter.Use(middleware.Logger())
		if options.AdminUsername != "" && options.AdminPassword != "" {
			adminRouter.Use(middleware.HTTPBasicAuth(options.AdminUsername, options.AdminPassword))
		}

		// Admin routes
		admin := adminRouter.Group("/admin")
		{
			admin.GET("/ping", ping)
			admin.GET("/version", version)
			admin.GET("/uptime", uptime)
			admin.GET("/stats", showStats)
			admin.GET("/dashboard", showDashboard)
			admin.GET("/dashboard/segmentKeys/:segment", showDashboardSegmentKeys)
		}

		adminRouter.Run(options.AdminPort)
	}()

	// API routes
	api := router.Group("/api")
	{
		api.GET("/splitChanges", splitChanges)
		api.GET("/segmentChanges/:name", segmentChanges)
		api.GET("/mySegments/:key", mySegments)
		api.POST("/testImpressions/bulk", postImpressionBulk(options.ImpressionListenerEnabled))
		api.POST("/metrics/times", postMetricsTimes)
		api.POST("/metrics/counters", postMetricsCounters)
		api.POST("/metrics/gauge", postMetricsGauge)
		api.POST("/metrics/time", postMetricsTime)
		api.POST("/metrics/counter", postMetricsCounter)
	}
	router.Run(options.Port)
}
