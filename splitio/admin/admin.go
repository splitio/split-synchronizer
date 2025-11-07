package admin

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/splitio/go-toolkit/v5/logging"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/admin/controllers"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	cstorage "github.com/splitio/split-synchronizer/v5/splitio/common/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services"

	"github.com/gin-gonic/gin"
)

const baseAdminPath = "/admin"
const baseInfoPath = "/info"
const baseShutdownPath = "/shutdown"

// Options encapsulates dependencies & config options for the Admin server
type Options struct {
	Host                string
	Port                int
	Name                string
	Proxy               bool
	Username            string
	Password            string
	Logger              logging.LoggerInterface
	Storages            adminCommon.Storages
	ImpressionsEvCalc   evcalc.Monitor
	EventsEvCalc        evcalc.Monitor
	Runtime             common.Runtime
	HcAppMonitor        application.MonitorIterface
	HcServicesMonitor   services.MonitorIterface
	Snapshotter         cstorage.Snapshotter
	TLS                 *tls.Config
	FullConfig          interface{}
	FlagSpecVersion     string
	LargeSegmentVersion string
	Hash                string
}

type AdminServer struct {
	server *http.Server
}

// NewServer instantiates a new admin server
func NewServer(options *Options) (*AdminServer, error) {
	router := gin.New()
	admin := router.Group(baseAdminPath)
	info := router.Group(baseInfoPath)
	shutdown := router.Group(baseShutdownPath)
	if options.Username != "" && options.Password != "" {
		admin = router.Group(baseAdminPath, gin.BasicAuth(gin.Accounts{options.Username: options.Password}))
		info = router.Group(baseInfoPath, gin.BasicAuth(gin.Accounts{options.Username: options.Password}))
		shutdown = router.Group(baseShutdownPath, gin.BasicAuth(gin.Accounts{options.Username: options.Password}))
	}

	dashboardController, err := controllers.NewDashboardController(
		options.Name,
		options.Proxy,
		options.Logger,
		options.Storages,
		options.ImpressionsEvCalc,
		options.EventsEvCalc,
		options.Runtime,
		options.HcAppMonitor,
		options.FlagSpecVersion,
		options.LargeSegmentVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("error instantiating dashboard controller: %w", err)
	}
	dashboardController.Register(admin)

	shutdownController := controllers.NewShutdownController(options.Runtime)
	shutdownController.Register(shutdown)

	healthcheckController := controllers.NewHealthCheckController(
		options.Logger,
		options.HcAppMonitor,
		options.HcServicesMonitor,
	)
	healthcheckController.Register(router)

	infoController := controllers.NewInfoController(options.Proxy, options.Runtime, options.FullConfig)
	infoController.Register(info)

	observabilityController, err := controllers.NewObservabilityController(options.Proxy, options.Logger, options.Storages)
	if err != nil {
		return nil, fmt.Errorf("error instantiating observability controller: %w", err)
	}
	observabilityController.Register(admin)

	if options.Snapshotter != nil {
		snapshotController := controllers.NewSnapshotController(options.Logger, options.Snapshotter, options.Hash)
		snapshotController.Register(admin)
	}

	return &AdminServer{
		server: &http.Server{
			Addr:      fmt.Sprintf("%s:%d", options.Host, options.Port),
			Handler:   router,
			TLSConfig: options.TLS,
		},
	}, nil
}

func (a *AdminServer) Start() error {
	if a.server.TLSConfig != nil {
		return a.server.ListenAndServeTLS("", "") // cert & key set in TLSConfig option
	}
	return a.server.ListenAndServe()
}
