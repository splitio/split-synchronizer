package proxy

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"strings"

	cfg "github.com/splitio/go-split-commons/v4/conf"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/go-split-commons/v4/tasks"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/admin"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	ssync "github.com/splitio/split-synchronizer/v5/splitio/common/sync"
	hcApplication "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
	hcAppCounter "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application/counter"
	hcServices "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services"
	hcServicesCounter "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services/counter"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	pTasks "github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

// Start initialize in proxy mode
func Start(logger logging.LoggerInterface, cfg *pconf.Main) error {

	clientKey, err := util.GetClientKey(cfg.Apikey)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error parsing client key from provided apikey: %w", err), common.ExitInvalidApikey)
	}

	// Initialization of DB
	var dbpath = persistent.BoltInMemoryMode
	if snapFile := cfg.Initialization.Snapshot; snapFile != "" {
		snap, err := snapshot.DecodeFromFile(snapFile)
		if err != nil {
			return fmt.Errorf("error parsing snapshot file: %w", err)
		}

		dbpath, err = snap.WriteDataToTmpFile()
		if err != nil {
			return fmt.Errorf("error writing temporary snapshot file: %w", err)
		}

		logger.Debug("Database created from snapshot at", dbpath)
	}

	dbInstance, err := persistent.NewBoltWrapper(dbpath, nil)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}

	// Set up the http proxy caching.
	// We need it fairly early since it's passed to the synchronizers, so that they can evict entries when a change is processed
	httpCache := caching.MakeProxyCache()

	// Getting initial config data
	advanced := cfg.BuildAdvancedConfig()
	metadata := util.GetMetadata(cfg.IPAddressEnabled, true)

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(cfg.Apikey, *advanced, logger, metadata)

	// Proxy storages already implement the observable interface, so no need to wrap them
	splitStorage := storage.NewProxySplitStorage(dbInstance, logger, cfg.Initialization.Snapshot != "")
	segmentStorage := storage.NewProxySegmentStorage(dbInstance, logger, cfg.Initialization.Snapshot != "")

	// Local telemetry
	tbufferSize := int(cfg.Sync.Advanced.TelemetryBuffer)
	tworkers := int(cfg.Sync.Advanced.TelemetryWorkers)

	// TODO(mredolatti) get these from config!
	width := int64(60) // seconds
	slices := 100      // max before rotation
	localTelemetryStorage := storage.NewTimeslicedProxyEndpointTelemetry(storage.NewProxyTelemetryFacade(), width, slices)

	// Healcheck Monitor
	splitsConfig, segmentsConfig := getAppCounterConfigs()
	appMonitor := hcApplication.NewMonitorImp(splitsConfig, segmentsConfig, nil, logger)
	servicesMonitor := hcServices.NewMonitorImp(getServicesCountersConfig(*advanced), logger)

	// Creating Workers and Tasks
	telemetryRecorder := api.NewHTTPTelemetryRecorder(cfg.Apikey, *advanced, logger)
	telemetryConfigTask := pTasks.NewTelemetryConfigFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)
	telemetryUsageTask := pTasks.NewTelemetryUsageFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)

	// impression bulks & counts - events
	ibufferSize := int(cfg.Sync.Advanced.ImpressionsBuffer)
	iworkers := int(cfg.Sync.Advanced.ImpressionsWorkers)
	impressionRecorder := api.NewHTTPImpressionRecorder(cfg.Apikey, *advanced, logger)
	impressionTask := pTasks.NewImpressionsFlushTask(impressionRecorder, logger, 1, ibufferSize, iworkers)
	impressionCountTask := pTasks.NewImpressionCountFlushTask(impressionRecorder, logger, 1, ibufferSize, iworkers)
	eventsRecorder := api.NewHTTPEventsRecorder(cfg.Apikey, *advanced, logger)
	eventsTask := pTasks.NewEventsFlushTask(eventsRecorder, logger, 1, int(cfg.Sync.Advanced.EventsBuffer), int(cfg.Sync.Advanced.EventsWorkers))

	// setup split, segments & local telemetry API interactions
	workers := synchronizer.Workers{
		SplitFetcher: caching.NewCacheAwareSplitSync(splitStorage, splitAPI.SplitFetcher, logger, localTelemetryStorage, httpCache, appMonitor),
		SegmentFetcher: caching.NewCacheAwareSegmentSync(splitStorage, segmentStorage, splitAPI.SegmentFetcher, logger, localTelemetryStorage, httpCache,
			appMonitor),
		TelemetryRecorder: telemetry.NewTelemetrySynchronizer(localTelemetryStorage, telemetryRecorder, splitStorage, segmentStorage, logger,
			metadata, localTelemetryStorage),
	}

	// setup periodic tasks in case streaming is disabled or we need to fall back to polling
	stasks := synchronizer.SplitTasks{
		SplitSyncTask: tasks.NewFetchSplitsTask(workers.SplitFetcher, int(cfg.Sync.SplitRefreshRateMs/1000), logger),
		SegmentSyncTask: tasks.NewFetchSegmentsTask(workers.SegmentFetcher, int(cfg.Sync.SegmentRefreshRateMs/1000), advanced.SegmentWorkers,
			advanced.SegmentQueueSize, logger),
		TelemetrySyncTask:        tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, int(cfg.Sync.Advanced.InternalMetricsRateMs), logger),
		ImpressionSyncTask:       impressionTask,
		ImpressionsCountSyncTask: impressionCountTask,
		EventSyncTask:            eventsTask,
	}

	// Creating Synchronizer for tasks
	sync := ssync.NewSynchronizer(*advanced, stasks, workers, logger, nil, []tasks.Task{telemetryConfigTask, telemetryUsageTask}, appMonitor)

	mstatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		sync,
		logger,
		*advanced,
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
				AdvancedConfig: *advanced,
				TaskPeriods: conf.TaskPeriods{
					SplitSync:     int(cfg.Sync.SplitRefreshRateMs / 1000),
					SegmentSync:   int(cfg.Sync.SegmentRefreshRateMs / 1000),
					TelemetrySync: int(cfg.Sync.Advanced.InternalMetricsRateMs / 1000),
				},
				ManagerConfig: conf.ManagerConfig{
					ListenerEnabled: cfg.Integrations.ImpressionListener.Endpoint != "",
				},
			},
			time.Since(before).Milliseconds(),
			map[string]int64{cfg.Apikey: 1},
			nil,
		)
	case synchronizer.Error:
		if cfg.Initialization.Snapshot == "" {
			// If we started from a snapshot, failure to sinchronize should not bring the app down
			logger.Error("Initial synchronization failed. Either split is unreachable or the APIKey is incorrect. Aborting execution.")
			return common.NewInitError(fmt.Errorf("error instantiating sync manager: %w", err), common.ExitTaskInitialization)
		}
		logger.Warning("Failed to perform initial sync with split servers but continuing from snapshot. Will keep retrying in BG")
	}

	rtm := common.NewRuntime(false, syncManager, logger, "Split Proxy", nil, nil, appMonitor, servicesMonitor)
	storages := adminCommon.Storages{
		SplitStorage:          splitStorage,
		SegmentStorage:        segmentStorage,
		LocalTelemetryStorage: localTelemetryStorage,
	}

	// --------------------------- ADMIN DASHBOARD ------------------------------
	cfgForAdmin := *cfg
	cfgForAdmin.Apikey = logging.ObfuscateAPIKey(cfgForAdmin.Apikey)
	adminServer, err := admin.NewServer(&admin.Options{
		Host:              cfg.Admin.Host,
		Port:              int(cfg.Admin.Port),
		Name:              "Split Proxy dashboard",
		Proxy:             true,
		Username:          cfg.Admin.Username,
		Password:          cfg.Admin.Password,
		Logger:            logger,
		Storages:          storages,
		Runtime:           rtm,
		Snapshotter:       dbInstance,
		HcAppMonitor:      appMonitor,
		HcServicesMonitor: servicesMonitor,
		FullConfig:        cfgForAdmin,
	})
	if err != nil {
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}
	go adminServer.ListenAndServe()

	proxyOptions := &Options{
		Host:                cfg.Server.Host,
		Port:                int(cfg.Server.Port),
		APIKeys:             cfg.Server.ClientApikeys,
		DebugOn:             strings.ToLower(cfg.Logging.Level) == "debug" || strings.ToLower(cfg.Logging.Level) == "verbose",
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

	if ilcfg := cfg.Integrations.ImpressionListener; ilcfg.Endpoint != "" {
		var err error
		proxyOptions.ImpressionListener, err = impressionlistener.NewImpressionBulkListener(ilcfg.Endpoint, int(ilcfg.QueueSize), nil)
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
