package admin

import (
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

const basepath = "/admin"

// Options encapsulates dependencies & config options for the Admin server
type Options struct {
	Host              string
	Port              int
	Name              string
	Proxy             bool
	Username          string
	Password          string
	Logger            logging.LoggerInterface
	Storages          adminCommon.Storages
	ImpressionsEvCalc evcalc.Monitor
	EventsEvCalc      evcalc.Monitor
	Runtime           common.Runtime
	HcAppMonitor      application.MonitorIterface
	HcServicesMonitor services.MonitorIterface
	Snapshotter       cstorage.Snapshotter
}

// NewServer instantiates a new admin server
func NewServer(options *Options) (*http.Server, error) {

	router := gin.New()
	admin := router.Group(basepath)
	dashboardController, err := controllers.NewDashboardController(
		options.Name,
		options.Proxy,
		options.Logger,
		options.Storages,
		options.ImpressionsEvCalc,
		options.EventsEvCalc,
		options.Runtime,
		options.HcAppMonitor,
	)
	if err != nil {
		return nil, fmt.Errorf("error instantiating dashboard controller: %w", err)
	}

	shutdownController := controllers.NewShutdownController(options.Runtime)
	shutdownController.Register(admin)

	dashboardController.Register(admin)
	// infoctrl, err := controllers.NewInfoController(options.Proxy, options.Runtime, options.Storages.LocalTelemetryStorage)
	// if err != nil {
	// 	return nil, fmt.Errorf("error instantiating info controller: %w", err)
	// }

	healthcheckController := controllers.NewHealthCheckController(
		options.Logger,
		options.HcAppMonitor,
		options.HcServicesMonitor,
	)

	healthcheckController.Register(router)
	if options.Snapshotter != nil {
		snapshotController := controllers.NewSnapshotController(options.Logger, options.Snapshotter)
		snapshotController.Register(admin)
	}

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", options.Host, options.Port),
		Handler: router,
	}, nil
}
