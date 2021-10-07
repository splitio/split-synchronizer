package admin

import (
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/v4/synchronizer/worker/impression"
	"github.com/splitio/go-toolkit/v5/logging"
	adminCommon "github.com/splitio/split-synchronizer/v4/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v4/splitio/admin/controllers"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services"

	"github.com/gin-gonic/gin"
)

const basepath = "/admin"

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
	ImpressionsRecorder impression.ImpressionRecorder
	EventRecorder       event.EventRecorder
	EventsEvCalc        evcalc.Monitor
	Runtime             common.Runtime
	HcAppMonitor        application.MonitorIterface
	HcServicesMonitor   services.MonitorIterface
}

// NewServer instantiates a new admin server
func NewServer(options *Options) (*http.Server, error) {

	router := gin.New()
	admin := router.Group(basepath)
	dataController := setupDataController(options, basepath)
	if dataController != nil {
		dataController.Register(admin)
	}

	dashboardController, err := controllers.NewDashboardController(
		options.Name,
		options.Proxy,
		options.Logger,
		options.Storages,
		options.ImpressionsEvCalc,
		options.EventsEvCalc,
		options.Runtime,
		dataController,
	)
	if err != nil {
		return nil, fmt.Errorf("error instantiating dashboard controller: %w", err)
	}

	dashboardController.Register(admin)
	// infoctrl, err := controllers.NewInfoController(options.Proxy, options.Runtime, options.Storages.LocalTelemetryStorage)
	// if err != nil {
	// 	return nil, fmt.Errorf("error instantiating info controller: %w", err)
	// }

	//router.GET("/admin/dashboard/segmentKeys/:segment", dctrl.SegmentKeys)

	healthcheckController := controllers.NewHealthCheckController(
		options.Logger,
		options.HcAppMonitor,
		options.HcServicesMonitor,
	)

	healthcheckController.Register(router.Group(""))

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", options.Host, options.Port),
		Handler: router,
	}, nil
}

func setupDataController(opts *Options, basepath string) *controllers.DataManagerController {
	if opts.Proxy {
		return nil
	}

	asImpressionDropper, idOk := opts.Storages.ImpressionStorage.(storage.DataDropper)
	asEventsDropper, edOk := opts.Storages.EventStorage.(storage.DataDropper)

	if !idOk || !edOk || opts.ImpressionsRecorder == nil || opts.EventRecorder == nil {
		return nil
	}

	return controllers.NewDataManagerController(
		asImpressionDropper,
		asEventsDropper,
		opts.ImpressionsRecorder,
		opts.EventRecorder,
		opts.Logger,
		basepath,
	)
}
