package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/web/admin/controllers"
	"github.com/splitio/split-synchronizer/v4/splitio/web/middleware"
)

// WebAdminOptions struct to set options for sync admin mode.
type WebAdminOptions struct {
	Port          int
	AdminUsername string
	AdminPassword string
	DebugOn       bool
}

// WebAdminServer web api for admin purpose
type WebAdminServer struct {
	options *WebAdminOptions
	router  *gin.Engine
}

// StartAdminWebAdmin starts new webserver
func StartAdminWebAdmin(options *WebAdminOptions, storages common.Storages, httpClients common.HTTPClients, recorders common.Recorders) {
	go func() {
		server := newWebAdminServer(options, storages, httpClients, recorders)
		server.Run()
	}()
}

func newWebAdminServer(options *WebAdminOptions, storages common.Storages, httpClients common.HTTPClients, recorders common.Recorders) *WebAdminServer {
	server := &WebAdminServer{options: options, router: gin.New()}
	server.router.Use(gin.Recovery())
	if !options.DebugOn {
		gin.SetMode(gin.ReleaseMode)
		server.router.Use(gin.LoggerWithWriter(gin.DefaultWriter, "/admin/healthcheck", "/admin/ping", "/admin/uptime", "/admin/version"))
	} else {
		server.router.Use(gin.Logger())
	}

	if options.AdminUsername != "" && options.AdminPassword != "" {
		server.router.Use(middleware.HTTPBasicAuth(options.AdminUsername, options.AdminPassword))
	}

	server.Router().Use(func(c *gin.Context) {
		c.Set(common.SplitStorage, storages.SplitStorage)
		c.Set(common.SegmentStorage, storages.SegmentStorage)
		c.Set(common.LocalMetricStorage, storages.LocalTelemetryStorage)
		c.Set(common.HTTPClientsGin, httpClients)
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			c.Set(common.RecordersGin, recorders)
			c.Set(common.EventStorage, storages.EventStorage)
			c.Set(common.ImpressionStorage, storages.ImpressionStorage)
		}
	})

	// Admin routes
	server.router.GET("/admin/ping", controllers.Ping)
	server.router.GET("/admin/version", controllers.Version)
	server.router.GET("/admin/uptime", controllers.Uptime)
	server.router.GET("/admin/stats", controllers.ShowStats)
	server.router.GET("/admin/stop/:stopType", controllers.StopProccess)
	server.router.GET("/admin/userConfig", controllers.GetConfiguration)
	server.Router().GET("/admin/healthcheck", controllers.HealthCheck)
	server.Router().GET("/admin/dashboard", controllers.Dashboard)
	server.Router().GET("/admin/dashboard/segmentKeys/:segment", controllers.DashboardSegmentKeys)
	server.Router().GET("/admin/metrics", controllers.GetMetrics)

	if appcontext.ExecutionMode() == appcontext.ProducerMode {
		server.Router().GET("/admin/events/queueSize", controllers.GetEventsQueueSize)
		server.Router().GET("/admin/impressions/queueSize", controllers.GetImpressionsQueueSize)
		server.Router().POST("/admin/events/drop/*size", controllers.DropEvents)
		server.Router().POST("/admin/impressions/drop/*size", controllers.DropImpressions)
		server.Router().POST("/admin/events/flush/*size", controllers.FlushEvents)
		server.Router().POST("/admin/impressions/flush/*size", controllers.FlushImpressions)
	}

	return server
}

// Router returns a pointer to router instance
func (w *WebAdminServer) Router() *gin.Engine {
	return w.router
}

// Run the webserver
func (w *WebAdminServer) Run() {
	w.router.Run(":" + strconv.Itoa(w.options.Port))
}
