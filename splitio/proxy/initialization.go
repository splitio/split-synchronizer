package proxy

import (
	"fmt"
	"log"
	"net/url"
	"time"

	cfg "github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/go-split-commons/v4/tasks"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/splitio/admin"
	adminCommon "github.com/splitio/split-synchronizer/v4/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/common/impressionlistener"
	ssync "github.com/splitio/split-synchronizer/v4/splitio/common/sync"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/evcalc"
	hcApplication "github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application"
	hcAppCounter "github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application/counter"
	hcServices "github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services"
	hcServicesCounter "github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services/counter"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/persistent"
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
	var dbpath = persistent.BoltInMemoryMode
	if conf.Data.Proxy.PersistMemoryPath != "" {
		dbpath = conf.Data.Proxy.PersistMemoryPath
	}
	dbInstance, err := persistent.NewBoltWrapper(dbpath, nil)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}

	// Set up the http proxy caching.
	// We need it fairly early since it's passed to the synchronizers, so that they can evict entries when a change is processed
	httpCache := caching.MakeProxyCache()

	// Getting initial config data
	advanced := conf.ParseAdvancedOptions()
	metadata := util.GetMetadata(true)

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(conf.Data.APIKey, advanced, logger, metadata)

	// Instantiating storages
	splitStorage := storage.NewProxySplitStorage(dbInstance, logger)
	segmentStorage := storage.NewProxySegmentStorage(dbInstance, logger)

	// Local telemetry
	localTelemetryStorage := storage.NewProxyTelemetryFacade()
	telemetryRecorder := api.NewHTTPTelemetryRecorder(conf.Data.APIKey, advanced, logger)
	telemetryConfigTask := pTasks.NewTelemetryConfigFlushTask(telemetryRecorder, logger, 5, 500, 2) // TODO(mredolatti): use proper config options!
	telemetryUsageTask := pTasks.NewTelemetryUsageFlushTask(telemetryRecorder, logger, 5, 500, 2)   // TODO(mredolatti): use proper config options!
	impressionRecorder := api.NewHTTPImpressionRecorder(conf.Data.APIKey, advanced, logger)

	impressionTask := pTasks.NewImpressionsFlushTask(impressionRecorder, logger, 5, 2, conf.Data.ImpressionsThreads)
	impressionCountTask := pTasks.NewImpressionCountFlushTask(impressionRecorder, logger, 5, 2, 1) // pass appropriate config
	eventsRecorder := api.NewHTTPEventsRecorder(conf.Data.APIKey, advanced, logger)
	eventsTask := pTasks.NewEventsFlushTask(eventsRecorder, logger, 5, 500, 2) // TODO(mredolatti): use proper config options.

	// Healcheck Monitor
	splitsConfig, segmentsConfig := getAppCounterConfigs()
	appMonitor := hcApplication.NewMonitorImp(splitsConfig, segmentsConfig, nil, logger)
	servicesMonitor := hcServices.NewMonitorImp(getServicesCountersConfig(advanced), logger)

	// Creating Workers and Tasks
	workers := synchronizer.Workers{
		//SplitFetcher:   fetcher.NewSplitFetcher(splitCollection, splitAPI.SplitFetcher, localTelemetryStorage, logger),
		//SegmentFetcher: fetcher.NewSegmentFetcher(segmentCollection, splitCollection, splitAPI.SegmentFetcher, localTelemetryStorage, logger),

		SplitFetcher:   caching.NewCacheAwareSplitSync(splitStorage, splitAPI.SplitFetcher, logger, localTelemetryStorage, httpCache, appMonitor),
		SegmentFetcher: caching.NewCacheAwareSegmentSync(splitStorage, segmentStorage, splitAPI.SegmentFetcher, logger, localTelemetryStorage, httpCache, appMonitor),

		TelemetryRecorder: telemetry.NewTelemetrySynchronizer(localTelemetryStorage, telemetryRecorder, splitStorage, segmentStorage, logger,
			metadata, localTelemetryStorage),
	}

	stasks := synchronizer.SplitTasks{
		SplitSyncTask: tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, logger),
		SegmentSyncTask: tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers,
			advanced.SegmentQueueSize, logger),
		TelemetrySyncTask:        tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsPostRate, logger),
		ImpressionSyncTask:       impressionTask,
		ImpressionsCountSyncTask: impressionCountTask,
		EventSyncTask:            eventsTask,
	}

	// Creating Synchronizer for tasks
	//sync := synchronizer.NewSynchronizer(advanced, stasks, workers, logger, nil)
	sync := ssync.NewSynchronizer(advanced, stasks, workers, logger, nil, []tasks.Task{telemetryConfigTask, telemetryUsageTask}, appMonitor)

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
		appMonitor,
	)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error instantiating sync manager: %w", err), common.ExitTaskInitialization)
	}

	// Run Sync Manager
	before := time.Now()
	go syncManager.Start()
	status := <-mstatus
	switch status {
	case synchronizer.Ready:
		logger.Info("Synchronizer tasks started")
		appMonitor.Start()
		servicesMonitor.Start()
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

	rtm := common.NewRuntime(false, syncManager, logger, conf.Data.Proxy.Title, nil, nil, appMonitor, servicesMonitor)
	impressionEvictionMonitor := evcalc.New(1) // TODO(mredolatti): set the correct thread count
	eventEvictionMonitor := evcalc.New(1)      // TODO(mredolatti): set the correct thread count

	storages := adminCommon.Storages{
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
		HcAppMonitor:      appMonitor,
		HcServicesMonitor: servicesMonitor,
	})
	if err != nil {
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}
	go adminServer.ListenAndServe()

	proxyOptions := &Options{
		Port:                conf.Data.Proxy.Port,
		APIKeys:             conf.Data.Proxy.Auth.APIKeys,
		DebugOn:             conf.Data.Logger.DebugOn,
		Logger:              logger,
		ProxySplitStorage:   splitStorage,
		SplitFetcher:        splitAPI.SplitFetcher,
		ProxySegmentStorage: segmentStorage,
		Telemetry:           localTelemetryStorage,
		ImpressionsSink:     impressionTask,
		ImpressionCountSink: impressionCountTask,
		EventsSink:          eventsTask,
		TelemetryConfigSink: telemetryConfigTask,
		TelemetryUsageSink:  telemetryUsageTask,
		Cache:               httpCache,
	}

	if conf.Data.ImpressionListener.Endpoint != "" {
		// TODO(mredolatti): make the listener queue size configurable
		var err error
		proxyOptions.ImpressionListener, err = impressionlistener.NewImpressionBulkListener(conf.Data.ImpressionListener.Endpoint, 20, nil)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error instantiating impression listener: %w", err), common.ExitTaskInitialization)
		}
		proxyOptions.ImpressionListener.Start()
	}

	proxyAPI := New(proxyOptions)
	go proxyAPI.Start()

	rtm.RegisterShutdownHandler()
	rtm.Block()
	return nil
}

func getAppCounterConfigs() (hcAppCounter.ThresholdConfig, hcAppCounter.ThresholdConfig) {
	splitsConfig := hcAppCounter.DefaultThresholdConfig("Splits")
	segmentsConfig := hcAppCounter.DefaultThresholdConfig("Segments")

	return splitsConfig, segmentsConfig
}

func getServicesCountersConfig(advanced cfg.AdvancedConfig) []hcServicesCounter.Config {
	var cfgs []hcServicesCounter.Config

	apiConfig := hcServicesCounter.DefaultConfig("API", advanced.SdkURL, "/version")
	eventsConfig := hcServicesCounter.DefaultConfig("Events", advanced.EventsURL, "/version")
	authConfig := hcServicesCounter.DefaultConfig("Auth", advanced.AuthServiceURL, "/health")

	telemetryURL, err := url.Parse(advanced.TelemetryServiceURL)
	if err != nil {
		log.Fatal(err)
	}
	telemetryConfig := hcServicesCounter.DefaultConfig("Telemetry", fmt.Sprintf("%s://%s", telemetryURL.Scheme, telemetryURL.Host), "/health")

	streamingURL, err := url.Parse(advanced.StreamingServiceURL)
	if err != nil {
		log.Fatal(err)
	}
	streamingConfig := hcServicesCounter.DefaultConfig("Streaming", fmt.Sprintf("%s://%s", streamingURL.Scheme, streamingURL.Host), "/health")

	return append(cfgs, telemetryConfig, authConfig, apiConfig, eventsConfig, streamingConfig)
}
