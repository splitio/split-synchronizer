package proxy

import (
	"fmt"
	"strconv"
	"time"

	cfg "github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/go-split-commons/v4/tasks"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/splitio/admin"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/common/impressionlistener"
	ssync "github.com/splitio/split-synchronizer/v4/splitio/common/sync"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/fetcher"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	pTasks "github.com/splitio/split-synchronizer/v4/splitio/proxy/tasks"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
)

// Start initialize in proxy mode
func Start(logger logging.LoggerInterface) error {

	clientKey, err := util.GetClientKey(conf.Data.APIKey)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error parsing client key from provided apikey: %w", err), common.ExitInvalidApikey)
	}

	// Initialization of DB
	var dbpath = boltdb.InMemoryMode
	if conf.Data.Proxy.PersistMemoryPath != "" {
		dbpath = conf.Data.Proxy.PersistMemoryPath
	}
	dbInstance, err := boltdb.NewInstance(dbpath, nil)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}

	// Getting initial config data
	advanced := conf.ParseAdvancedOptions()
	metadata := util.GetMetadata(true)

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(conf.Data.APIKey, advanced, logger, metadata)

	// Instantiating storages
	splitCollection := collections.NewSplitChangesCollection(dbInstance, logger)
	splitStorage := storage.NewSplitStorage(splitCollection)
	segmentCollection := collections.NewSegmentChangesCollection(dbInstance, logger)
	segmentStorage := storage.NewSegmentStorage(segmentCollection)

	// Local telemetry
	localTelemetryStorage := storage.NewProxyTelemetryFacade()
	telemetryRecorder := api.NewHTTPTelemetryRecorder(conf.Data.APIKey, advanced, logger)
	telemetryConfigTask := pTasks.NewTelemetryConfigFlushTask(telemetryRecorder, logger, 5, 500, 2) // TODO(mredolatti): use proper config options!
	telemetryUsageTask := pTasks.NewTelemetryUsageFlushTask(telemetryRecorder, logger, 5, 500, 2)   // TODO(mredolatti): use proper config options!
	impressionRecorder := api.NewHTTPImpressionRecorder(conf.Data.APIKey, advanced, logger)

	var impListener impressionlistener.ImpressionBulkListener
	if conf.Data.ImpressionListener.Endpoint != "" {
		// TODO(mredolatti): make the listener queue size configurable
		var err error
		impListener, err = impressionlistener.NewImpressionBulkListener(conf.Data.ImpressionListener.Endpoint, 20, nil)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error instantiating impression listener: %w", err), common.ExitTaskInitialization)
		}

	}
	impressionTask := pTasks.NewImpressionsFlushTask(impressionRecorder, logger, 5, 2, conf.Data.ImpressionsThreads, impListener)
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
		return common.NewInitError(fmt.Errorf("error instantiating sync manager: %w", err), common.ExitTaskInitialization)
	}

	// Proxy mode - graceful shutdown// TODO!
	// go gracefulShutdownProxy(sigs, gracefulShutdownWaitingGroup, syncManager)

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
					ListenerEnabled: false, // listener is not by impression, this is not needed in split-sync
				},
			},
			time.Now().Sub(before).Milliseconds(),
			map[string]int64{conf.Data.APIKey: 1},
			nil,
		)
	case synchronizer.Error:
		logger.Error("Initial synchronization failed. Either split is unreachable or the APIKey is incorrect. Aborting execution.")
		return common.NewInitError(fmt.Errorf("error instantiating sync manager: %w", err), common.ExitTaskInitialization)
	}

	// TODO(mredolatti): setup impression listener properly
	// if conf.Data.ImpressionListener.Endpoint != "" {
	// 	go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
	// 		Endpoint: conf.Data.ImpressionListener.Endpoint,
	// 	})
	// }

	rtm := common.NewRuntime(false, syncManager, logger, nil, nil)
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
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
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
	go proxyAPI.Start()

	//
	rtm.RegisterShutdownHandler()
	rtm.Block()
	return nil
}
