package proxy

import (
	"fmt"
	"net/http"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers"
	proxyMW "github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers/middleware"
	proxyStorage "github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/tasks"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// Options struct to set options for Proxy mode.
type Options struct {
	Logger                  logging.LoggerInterface
	Port                    string
	APIKeys                 []string
	ImpressionListener      string
	DebugOn                 bool
	SplitBoltDBCollection   *collections.SplitChangesCollection
	SegmentBoltDBCollection *collections.SegmentChangesCollection
	ImpressionsSink         tasks.DeferredRecordingTask
	EventsSink              tasks.DeferredRecordingTask
	TelemetryConfigSink     tasks.DeferredRecordingTask
	TelemetryUsageSink      tasks.DeferredRecordingTask
	Telemetry               proxyStorage.ProxyEndpointTelemetry
}

// API bundles all components required to answer API calls from split sdks
type API struct {
	server              *http.Server
	sdkConroller        *controllers.SdkServerController
	eventsConroller     *controllers.EventsServerController
	telemetryController *controllers.TelemetryServerController
}

// Start the Proxy service endpoints
func (s *API) Start() error {
	return s.server.ListenAndServe()
}

// New instantiates a new Server
func New(options *Options) *API {
	if !options.DebugOn {
		gin.SetMode(gin.ReleaseMode)
	}

	apikeyValidator := proxyMW.NewAPIKeyValidator(options.APIKeys)

	sdkController := setupSdkController(options)
	eventsController := setupEventsController(options, apikeyValidator)
	telemetryController := setupTelemetryController(options)

	router := gin.New()
	router.Use(gin.Recovery())

	//CORS - Allows all origins
	router.Use(setupCorsMiddleware())

	// API routes
	api := router.Group("/api")
	api.Use(proxyMW.NewProxyLatencyMiddleware(options.Telemetry).Track)
	api.Use(apikeyValidator.AsMiddleware)
	api.Use(gzip.Gzip(gzip.DefaultCompression))
	// api.GET("/auth", auth)
	api.GET("/splitChanges", sdkController.SplitChanges)
	api.GET("/segmentChanges/:name", sdkController.SegmentChanges)
	api.GET("/mySegments/:key", sdkController.MySegments)
	api.POST("/events/bulk", eventsController.EventsBulk)
	api.POST("/testImpressions/bulk", eventsController.TestImpressionsBulk)
	api.POST("/testImpressions/count", eventsController.TestImpressionsCount)
	api.POST("/metrics/config", telemetryController.Config)
	api.POST("/metrics/usage", telemetryController.Usage)
	api.POST("/metrics/times", eventsController.DummyAlwaysOk)
	api.POST("/metrics/counters", eventsController.DummyAlwaysOk)
	api.POST("/metrics/gauge", eventsController.DummyAlwaysOk)
	api.POST("/metrics/time", eventsController.DummyAlwaysOk)
	api.POST("/metrics/counter", eventsController.DummyAlwaysOk)

	// Beacon routes
	beacon := router.Group("/api")
	beacon.POST("/testImpressions/count/beacon", eventsController.TestImpressionsCountBeacon)
	beacon.POST("/testImpressions/beacon", eventsController.TestImpressionsBeacon)
	beacon.POST("/events/beacon", eventsController.EventsBulkBeacon)

	return &API{
		server: &http.Server{
			Addr:    fmt.Sprintf("0.0.0.0%s", options.Port),
			Handler: router,
		},
		sdkConroller:        sdkController,
		eventsConroller:     eventsController,
		telemetryController: telemetryController,
	}
}

func setupSdkController(options *Options) *controllers.SdkServerController {
	return controllers.NewSdkServerController(
		options.Logger,
		options.SplitBoltDBCollection,
		options.SegmentBoltDBCollection,
		options.Telemetry,
	)
}

func setupEventsController(options *Options, apikeyValidator *proxyMW.APIKeyValidator) *controllers.EventsServerController {
	return controllers.NewEventsServerController(
		options.Logger,
		options.Telemetry,
		options.ImpressionsSink,
		options.EventsSink,
		nil, // TODO(mredolatti): add impressions listener
		apikeyValidator.IsValid,
	)
}

func setupTelemetryController(options *Options) *controllers.TelemetryServerController {
	return controllers.NewTelemetryServerController(
		options.Logger,
		options.Telemetry,
		options.TelemetryConfigSink,
		options.TelemetryUsageSink,
	)
}

func setupCorsMiddleware() func(*gin.Context) {
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"SplitSDKMachineName",
		"SplitSDKMachineIP",
		"SplitSDKVersion",
		"Authorization",
	}
	return cors.New(corsConfig)
}
