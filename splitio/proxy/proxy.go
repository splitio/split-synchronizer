package proxy

import (
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/controllers/middleware"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/splitio/gincache"
)

// Options struct to set options for Proxy mode.
type Options struct {
	// Logger to propagate everywhere
	Logger logging.LoggerInterface

	// Host to where incoming http connections will be listened
	Host string

	// HTTP port to use for the server
	Port int

	// APIKeys used for authenticating proxy requests
	APIKeys []string

	// ImpressionListener to forward incoming impression bulks to
	ImpressionListener impressionlistener.ImpressionBulkListener

	// Whether to do verbose logging in the gin framework
	DebugOn bool

	// used for on-demand splitchanges fetching when a requested summary is not cached
	SplitFetcher service.SplitFetcher

	// used to resolve splitChanges requests
	ProxySplitStorage storage.ProxySplitStorage

	// used to resolve segmentChanges & mySegments requests
	ProxySegmentStorage storage.ProxySegmentStorage

	// what to do with received impression bulk payloads
	ImpressionsSink tasks.DeferredRecordingTask

	// what to do with received impression count payloads
	ImpressionCountSink tasks.DeferredRecordingTask

	// what to do with received event bulk payloads
	EventsSink tasks.DeferredRecordingTask

	// what to do with incoming telemetry.config payloads
	TelemetryConfigSink tasks.DeferredRecordingTask

	// what to do with incoming telemetry.runtime payloads
	TelemetryUsageSink tasks.DeferredRecordingTask

	// what to do with incoming telemetry.keys/cs payloads
	TelemetryKeysClientSideSink tasks.DeferredRecordingTask

	// what to do with incoming telemetry.keys/ss payloads
	TelemetryKeysServerSideSink tasks.DeferredRecordingTask

	// used to record local metrics
	Telemetry storage.ProxyEndpointTelemetry

	Cache *gincache.Middleware
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

	apikeyValidator := middleware.NewAPIKeyValidator(options.APIKeys)
	authController := controllers.NewAuthServerController()
	sdkController := setupSdkController(options)
	eventsController := setupEventsController(options, apikeyValidator)
	telemetryController := setupTelemetryController(options, apikeyValidator)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(setupCorsMiddleware())
	router.Use(middleware.SetEndpoint)
	router.Use(middleware.NewProxyMetricsMiddleware(options.Telemetry).Track)

	// split the main router into regular & beacon endpoints
	regular := router.Group("/api")
	regular.Use(apikeyValidator.AsMiddleware)
	regular.Use(gzip.Gzip(gzip.DefaultCompression))

	// Beacon endpoints group
	beacon := router.Group("/api")

	var cacheableRouter gin.IRouter = regular
	// If we got a cache in the options, fork the router, add the caching middleware,
	// and pass it to Auth & Sdk controllers
	if options.Cache != nil {
		cacheableRouter = router.Group("/api")
		cacheableRouter.Use(apikeyValidator.AsMiddleware)
		cacheableRouter.Use(options.Cache.Handle)
		cacheableRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	}
	authController.Register(cacheableRouter)
	sdkController.Register(cacheableRouter)
	eventsController.Register(regular, beacon)
	telemetryController.Register(regular, beacon)

	return &API{
		server:              &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", options.Port), Handler: router},
		sdkConroller:        sdkController,
		eventsConroller:     eventsController,
		telemetryController: telemetryController,
	}
}

func setupSdkController(options *Options) *controllers.SdkServerController {
	return controllers.NewSdkServerController(
		options.Logger,
		options.SplitFetcher,
		options.ProxySplitStorage,
		options.ProxySegmentStorage,
	)
}

func setupEventsController(options *Options, apikeyValidator *middleware.APIKeyValidator) *controllers.EventsServerController {
	return controllers.NewEventsServerController(
		options.Logger,
		options.ImpressionsSink,
		options.ImpressionCountSink,
		options.EventsSink,
		options.ImpressionListener,
		apikeyValidator.IsValid,
	)
}

func setupTelemetryController(options *Options, apikeyValidator *middleware.APIKeyValidator) *controllers.TelemetryServerController {
	return controllers.NewTelemetryServerController(
		options.Logger,
		options.TelemetryConfigSink,
		options.TelemetryUsageSink,
		options.TelemetryKeysClientSideSink,
		options.TelemetryKeysServerSideSink,
		apikeyValidator.IsValid,
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
		"SplitSDKImpressionsMode",
		"Authorization",
	}
	return cors.New(corsConfig)
}
