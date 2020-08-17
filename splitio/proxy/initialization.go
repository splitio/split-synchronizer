package proxy

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/synchronizer"
	"github.com/splitio/go-split-commons/synchronizer/worker/metric"
	"github.com/splitio/go-split-commons/tasks"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/common"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/proxy/fetcher"
	"github.com/splitio/split-synchronizer/splitio/proxy/interfaces"
	"github.com/splitio/split-synchronizer/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/util"
)

func gracefulShutdownProxy(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager *synchronizer.Manager) {
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

	// SyncManager
	syncManager.Stop()

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()
	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

// Start initialize in proxy mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	var dbpath = boltdb.InMemoryMode
	if conf.Data.Proxy.PersistMemoryPath != "" {
		dbpath = conf.Data.Proxy.PersistMemoryPath
	}
	boltdb.Initialize(dbpath, nil)
	interfaces.Initialize()

	advanced := util.ParseAdvancedOptions()
	metadata := dtos.Metadata{
		MachineIP:   "NA",
		MachineName: "NA",
		SDKVersion:  "split-sync-proxy-" + splitio.Version,
	}

	// Setup fetchers & recorders
	splitAPI := service.NewSplitAPI(
		conf.Data.APIKey,
		advanced,
		log.Instance,
		metadata,
	)

	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	splitStorage := storage.NewSplitStorage(splitCollection)
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segmentStorage := storage.NewSegmentStorage(segmentCollection)

	workers := synchronizer.Workers{
		SplitFetcher:      fetcher.NewSplitFetcher(splitCollection, splitAPI.SplitFetcher, interfaces.ProxyTelemetryWrapper, log.Instance),
		SegmentFetcher:    fetcher.NewSegmentFetcher(segmentCollection, splitCollection, splitAPI.SegmentFetcher, interfaces.ProxyTelemetryWrapper, log.Instance),
		TelemetryRecorder: metric.NewRecorderSingle(interfaces.TelemetryStorage, splitAPI.MetricRecorder, metadata),
	}
	splitTasks := synchronizer.SplitTasks{
		SplitSyncTask:     tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, log.Instance),
		SegmentSyncTask:   tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers, advanced.SegmentQueueSize, log.Instance),
		TelemetrySyncTask: tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsPostRate, log.Instance),
	}
	syncImpl := synchronizer.NewSynchronizer(
		advanced,
		splitTasks,
		workers,
		log.Instance,
		nil,
	)

	managerStatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		syncImpl,
		log.Instance,
		advanced,
		splitAPI.AuthClient,
		splitStorage,
		managerStatus,
	)
	if err != nil {
		panic(err)
	}

	go gracefulShutdownProxy(sigs, gracefulShutdownWaitingGroup, syncManager)

	go syncManager.Start()
	select {
	case status := <-managerStatus:
		switch status {
		case synchronizer.Ready:
			log.Instance.Info("Synchronizer tasks started")
		case synchronizer.Error:
			os.Exit(splitio.ExitTaskInitialization)
		}
	}

	if conf.Data.ImpressionListener.Endpoint != "" {
		go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
			Endpoint: conf.Data.ImpressionListener.Endpoint,
		})
	}

	httpClients := common.HTTPClients{
		SdkClient:    api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.SdkURL, log.Instance, metadata),
		EventsClient: api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.EventsURL, log.Instance, metadata),
	}
	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, splitStorage, httpClients.SdkClient, httpClients.EventsClient)

	controllers.InitializeImpressionWorkers(
		conf.Data.Proxy.ImpressionsMaxSize,
		int64(conf.Data.ImpressionsPostRate),
		gracefulShutdownWaitingGroup,
	)
	controllers.InitializeEventWorkers(
		conf.Data.Proxy.EventsMaxSize,
		int64(conf.Data.EventsPostRate),
		gracefulShutdownWaitingGroup,
	)

	proxyOptions := &Options{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   conf.Data.Proxy.Auth.APIKeys,
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
		httpClients:               httpClients,
		splitStorage:              splitStorage,
		segmentStorage:            segmentStorage,
	}

	// Run webserver loop
	Run(proxyOptions)
}
