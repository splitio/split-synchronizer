package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/splitio/web/admin/controllers"
	"github.com/splitio/split-synchronizer/splitio/web/middleware"
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

// NewWebAdminServer creates a webserver
func NewWebAdminServer(options *WebAdminOptions) *WebAdminServer {
	if !options.DebugOn {
		gin.SetMode(gin.ReleaseMode)
	}

	server := &WebAdminServer{options: options, router: gin.New()}
	server.router.Use(gin.Recovery())
	server.router.Use(gin.Logger())

	if options.AdminUsername != "" && options.AdminPassword != "" {
		server.router.Use(middleware.HTTPBasicAuth(options.AdminUsername, options.AdminPassword))
	}

	// Admin routes
	server.router.GET("/admin/ping", controllers.Ping)
	server.router.GET("/admin/version", controllers.Version)
	server.router.GET("/admin/uptime", controllers.Uptime)

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
