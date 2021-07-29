package proxy

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/interfaces"
	"github.com/splitio/split-synchronizer/v4/splitio/web/admin"
	"github.com/splitio/split-synchronizer/v4/splitio/web/middleware"
)

// Options struct to set options for Proxy mode.
type Options struct {
	Port                      string
	AdminPort                 int
	AdminUsername             string
	AdminPassword             string
	APIKeys                   []string
	ImpressionListenerEnabled bool
	DebugOn                   bool
	splitStorage              storage.SplitStorage
	segmentStorage            storage.SegmentStorage
	httpClients               common.HTTPClients
}

// Run runs the proxy server
func Run(options *Options) {
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
	router.Use(middleware.Logger())

	// WebAdmin configuration
	waOptions := &admin.WebAdminOptions{
		Port:          options.AdminPort,
		AdminUsername: options.AdminUsername,
		AdminPassword: options.AdminPassword,
		DebugOn:       options.DebugOn,
	}

	admin.StartAdminWebAdmin(waOptions, common.Storages{
		SplitStorage:          options.splitStorage,
		SegmentStorage:        options.segmentStorage,
		LocalTelemetryStorage: interfaces.LocalTelemetry,
	}, options.httpClients, common.Recorders{})

	// Beacon routes
	beacon := router.Group("/api")
	{
		beacon.POST("/testImpressions/count/beacon", postImpressionsCountBeacon(options.APIKeys))
		beacon.POST("/testImpressions/beacon", postImpressionBeacon(options.APIKeys, options.ImpressionListenerEnabled))
		beacon.POST("/events/beacon", postEventsBeacon(options.APIKeys))
	}

	// API routes
	api := router.Group("/api")
	api.Use(middleware.ValidateAPIKeys(options.APIKeys))
	api.Use(gzip.Gzip(gzip.DefaultCompression))
	{
		api.GET("/splitChanges", splitChanges)
		api.GET("/segmentChanges/:name", segmentChanges)
		api.GET("/mySegments/:key", mySegments)
		api.POST("/testImpressions/bulk", postImpressionBulk(options.ImpressionListenerEnabled))
		api.POST("/testImpressions/count", postImpressionsCount())
		api.POST("/metrics/times", postMetricsTimes)
		api.POST("/metrics/counters", postMetricsCounters)
		api.POST("/metrics/gauge", postMetricsGauge)
		api.POST("/metrics/time", postMetricsTime)
		api.POST("/metrics/counter", postMetricsCounter)
		api.POST("/events/bulk", postEvents)
		api.GET("/auth", auth)
	}
	router.Run(options.Port)
}
