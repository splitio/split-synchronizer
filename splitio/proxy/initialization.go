package proxy

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/task"
)

func gracefulShutdownProxy(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Events - Emit task stop signal
	fmt.Println(" -> Sending STOP to impression posting goroutine")
	controllers.StopEventsRecording()

	// Impressions - Emit task stop signal
	fmt.Println(" -> Sending STOP to event posting goroutine")
	controllers.StopImpressionsRecording()

	// Healthcheck - Emit task stop signal
	fmt.Println(" -> Sending STOP to healthcheck goroutine")
	task.StopHealtcheck()

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()
	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

// Start initialize in proxy mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	go gracefulShutdownProxy(sigs, gracefulShutdownWaitingGroup)
	go task.FetchRawSplits(conf.Data.SplitsFetchRate, conf.Data.SegmentFetchRate)

	if conf.Data.ImpressionListener.Endpoint != "" {
		go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
			Endpoint: conf.Data.ImpressionListener.Endpoint,
		})
	}

	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, nil)

	controllers.InitializeImpressionWorkers(
		conf.Data.Proxy.ImpressionsMaxSize,
		int64(conf.Data.ImpressionsPostRate),
		gracefulShutdownWaitingGroup,
	)
	controllers.InitializeEventWorkers(
		conf.Data.Proxy.EventsMaxSize,
		int64(conf.Data.EventsPushRate),
		gracefulShutdownWaitingGroup,
	)

	proxyOptions := &ProxyOptions{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   conf.Data.Proxy.Auth.APIKeys,
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
	}

	//Run webserver loop
	Run(proxyOptions)
}
