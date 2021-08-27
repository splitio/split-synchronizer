package proxy

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	cfg "github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/go-split-commons/v4/tasks"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/admin"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	ssync "github.com/splitio/split-synchronizer/v4/splitio/common/sync"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/fetcher"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	pTasks "github.com/splitio/split-synchronizer/v4/splitio/proxy/tasks"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
)

func gracefulShutdownProxy(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager synchronizer.Manager) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Events - Emit task stop signal
	fmt.Println(" -> Sending STOP to impression posting goroutine")
	// TODO(mredolatti): Setup this flushing

	//controllers.StopEventsRecording()

	// Impressions - Emit task stop signal
	fmt.Println(" -> Sending STOP to event posting goroutine")
	// controllers.StopImpressionsRecording()

	// Healthcheck - Emit task stop signal
	fmt.Println(" -> Sending STOP to healthcheck goroutine")
	task.StopHealtcheck()

	// Stopping Sync Manager in charge of PeriodicFetchers and PeriodicRecorders as well as Streaming
	fmt.Println(" -> Sending STOP to Synchronizer")
	syncManager.Stop()

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()

	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

// Start initialize in proxy mode
func Start(logger logging.LoggerInterface, sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) error {

	clientKey, err := util.GetClientKey(conf.Data.APIKey)
	if err != nil {
		logger.Error(err)
		return fmt.Errorf("error parsing client key from provided apikey: %w", err)
	}

	// Initialization of DB
	var dbpath = boltdb.InMemoryMode
	if conf.Data.Proxy.PersistMemoryPath != "" {
		dbpath = conf.Data.Proxy.PersistMemoryPath
	}
	dbInstance, err := boltdb.NewInstance(dbpath, nil)
	if err != nil {
		return fmt.Errorf("error instantiating boltdb: %w", err)
	}

	// Getting initial config data
	advanced := conf.ParseAdvancedOptions()
	metadata := util.GetMetadata()

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(conf.Data.APIKey, advanced, logger, metadata)

	// Instantiating storages
	splitCollection := collections.NewSplitChangesCollection(dbInstance)
	splitStorage := storage.NewSplitStorage(splitCollection)
	segmentCollection := collections.NewSegmentChangesCollection(dbInstance)
	segmentStorage := storage.NewSegmentStorage(segmentCollection)

	// Local telemetry
	localTelemetryStorage := storage.NewProxyTelemetryFacade()
	telemetryRecorder := api.NewHTTPTelemetryRecorder(conf.Data.APIKey, advanced, logger)
	telemetryConfigTask := pTasks.NewTelemetryConfigFlushTask(telemetryRecorder, logger, 5, 500, 2) // TODO(mredolatti): use proper config options!
	telemetryUsageTask := pTasks.NewTelemetryUsageFlushTask(telemetryRecorder, logger, 5, 500, 2)   // TODO(mredolatti): use proper config options!
	impressionRecorder := api.NewHTTPImpressionRecorder(conf.Data.APIKey, advanced, logger)
	impressionTask := pTasks.NewImpressionsFlushTask(impressionRecorder, logger, 5, 2, conf.Data.ImpressionsThreads)
	eventsRecorder := api.NewHTTPEventsRecorder(conf.Data.APIKey, advanced, logger)
	eventsTask := pTasks.NewEventsFlushTask(eventsRecorder, logger, 5, 500, 2) // TODO(mredolatti): use proper config options.

	// Creating Workers and Tasks
	workers := synchronizer.Workers{
		SplitFetcher:   fetcher.NewSplitFetcher(splitCollection, splitAPI.SplitFetcher, localTelemetryStorage, logger),
		SegmentFetcher: fetcher.NewSegmentFetcher(segmentCollection, splitCollection, splitAPI.SegmentFetcher, localTelemetryStorage, logger),
		TelemetryRecorder: telemetry.NewTelemetrySynchronizer(localTelemetryStorage, telemetryRecorder, splitStorage, segmentStorage, logger,
			metadata, localTelemetryStorage),
	}

	stasks := synchronizer.SplitTasks{
		SplitSyncTask: tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, logger),
		SegmentSyncTask: tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers,
			advanced.SegmentQueueSize, logger),
		TelemetrySyncTask:  tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsPostRate, logger),
		ImpressionSyncTask: impressionTask,
		EventSyncTask:      eventsTask,
	}

	// Creating Synchronizer for tasks
	//sync := synchronizer.NewSynchronizer(advanced, stasks, workers, logger, nil)
	sync := ssync.NewSynchronizer(advanced, stasks, workers, logger, nil, []tasks.Task{telemetryConfigTask, telemetryUsageTask})

	mstatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		sync,
		logger,
		advanced,
		splitAPI.AuthClient,
		splitStorage,
		mstatus,
		localTelemetryStorage,
		metadata,
		&clientKey,
	)
	if err != nil {
		panic(err)
	}

	// Proxy mode - graceful shutdown
	go gracefulShutdownProxy(sigs, gracefulShutdownWaitingGroup, syncManager)

	// Run Sync Manager
	before := time.Now()
	go syncManager.Start()
	status := <-mstatus
	switch status {
	case synchronizer.Ready:
		logger.Info("Synchronizer tasks started")
		workers.TelemetryRecorder.SynchronizeConfig(
			telemetry.InitConfig{
				AdvancedConfig: advanced,
				TaskPeriods: cfg.TaskPeriods{
					SplitSync:      conf.Data.SplitsFetchRate,
					SegmentSync:    conf.Data.SegmentFetchRate,
					ImpressionSync: conf.Data.ImpressionsPostRate,
					TelemetrySync:  10, // TODO(mredolatti): Expose this as a config option
				},
				ManagerConfig: cfg.ManagerConfig{
					ImpressionsMode: conf.Data.ImpressionsMode,
					ListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
				},
			},
			time.Now().Sub(before).Milliseconds(),
			map[string]int64{conf.Data.APIKey: 1},
			nil,
		)
	case synchronizer.Error:
		logger.Error("Initial synchronization failed. Either split is unreachable or the APIKey is incorrect. Aborting execution.")
		os.Exit(splitio.ExitTaskInitialization)
	}

	// TODO(mredolatti): setup impression listener properly
	// if conf.Data.ImpressionListener.Endpoint != "" {
	// 	go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
	// 		Endpoint: conf.Data.ImpressionListener.Endpoint,
	// 	})
	// }

	rtm := common.NewRuntime()
	impressionEvictionMonitor := evcalc.New(1) // TODO(mredolatti): set the correct thread count
	eventEvictionMonitor := evcalc.New(1)      // TODO(mredolatti): set the correct thread count

	storages := common.Storages{
		SplitStorage:          splitStorage,
		SegmentStorage:        segmentStorage,
		LocalTelemetryStorage: localTelemetryStorage,
	}

	// --------------------------- ADMIN DASHBOARD ------------------------------
	adminServer, err := admin.NewServer(&admin.Options{
		Host:              "0.0.0.0",
		Port:              conf.Data.Proxy.AdminPort,
		Name:              "Split Synchronizer dashboard (producer mode)",
		Proxy:             true,
		Username:          conf.Data.Proxy.AdminUsername,
		Password:          conf.Data.Proxy.AdminPassword,
		Logger:            logger,
		Storages:          storages,
		ImpressionsEvCalc: impressionEvictionMonitor,
		EventsEvCalc:      eventEvictionMonitor,
		Runtime:           rtm,
	})
	if err != nil {
		panic(err.Error())
	}
	go adminServer.ListenAndServe()

	proxyOptions := &Options{
		Port:                    ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                 conf.Data.Proxy.Auth.APIKeys,
		DebugOn:                 conf.Data.Logger.DebugOn,
		Logger:                  logger,
		SplitBoltDBCollection:   &splitCollection,
		SegmentBoltDBCollection: &segmentCollection,
		Telemetry:               localTelemetryStorage,
		ImpressionsSink:         impressionTask,
	}

	proxyAPI := New(proxyOptions)
	return proxyAPI.Start()

	// TODO(mredolatti): configure and start webadmin
	// AdminPort:                 conf.Data.Proxy.AdminPort,
	// AdminUsername:             conf.Data.Proxy.AdminUsername,
	// AdminPassword:             conf.Data.Proxy.AdminPassword,
	// ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
	// httpClients:    httpClients,
	// splitStorage:   splitStorage,
	// segmentStorage: segmentStorage,
	// latencyStorage: localTelemetryStorage,

	// go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, splitStorage, httpClients)

}
