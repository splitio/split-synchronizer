package admin

import (
	"fmt"
	"net/http"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/admin/controllers"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"

	"github.com/gin-gonic/gin"
)

type Options struct {
	Host              string
	Port              int
	Name              string
	Proxy             bool
	Username          string
	Password          string
	Logger            logging.LoggerInterface
	Storages          common.Storages
	ImpressionsEvCalc evcalc.Monitor
	EventsEvCalc      evcalc.Monitor
	Runtime           common.Runtime
}

func NewServer(options *Options) (*http.Server, error) {
	dctrl, err := controllers.NewDashboardController(
		options.Name,
		options.Proxy,
		options.Logger,
		options.Storages,
		options.ImpressionsEvCalc,
		options.EventsEvCalc,
		options.Runtime,
	)
	if err != nil {
		return nil, fmt.Errorf("error instantiating dashboard controller: %w", err)
	}

	// infoctrl, err := controllers.NewInfoController(options.Proxy, options.Runtime, options.Storages.LocalTelemetryStorage)
	// if err != nil {
	// 	return nil, fmt.Errorf("error instantiating info controller: %w", err)
	// }

	router := gin.New()
	admin := router.Group("/admin")
	dctrl.Register(admin)
	//router.GET("/admin/dashboard/segmentKeys/:segment", dctrl.SegmentKeys)

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", options.Host, options.Port),
		Handler: router,
	}, nil
}
