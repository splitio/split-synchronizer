package proxy

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"strings"

	"github.com/splitio/go-split-commons/v5/conf"
	"github.com/splitio/go-split-commons/v5/flagsets"
	"github.com/splitio/go-split-commons/v5/service/api"
	"github.com/splitio/go-split-commons/v5/synchronizer"
	"github.com/splitio/go-split-commons/v5/tasks"
	"github.com/splitio/go-split-commons/v5/telemetry"
	"github.com/splitio/go-toolkit/v5/backoff"
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
	advanced.FlagSetsFilter = cfg.FlagSetsFilter
	advanced.AuthSpecVersion = cfg.SpecVersion
	advanced.FlagsSpecVersion = cfg.SpecVersion
	metadata := util.GetMetadata(cfg.IPAddressEnabled, true)

	// FlagSetsFilter
	flagSetsFilter := flagsets.NewFlagSetFilter(cfg.FlagSetsFilter)

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(cfg.Apikey, *advanced, logger, metadata)

	// Proxy storages already implement the observable interface, so no need to wrap them
	splitStorage := storage.NewProxySplitStorage(dbInstance, logger, flagsets.NewFlagSetFilter(cfg.FlagSetsFilter), cfg.Initialization.Snapshot != "")
	segmentStorage := storage.NewProxySegmentStorage(dbInstance, logger, cfg.Initialization.Snapshot != "")

	// Local telemetry
	tbufferSize := int(cfg.Sync.Advanced.TelemetryBuffer)
	tworkers := int(cfg.Sync.Advanced.TelemetryWorkers)

	localTelemetryStorage := storage.NewTimeslicedProxyEndpointTelemetry(
		storage.NewProxyTelemetryFacade(),
		cfg.Observability.TimeSliceWidthSecs,
		int(cfg.Observability.MaxTimeSliceCount),
	)

	// Healcheck Monitor
	splitsConfig, segmentsConfig := getAppCounterConfigs()
	appMonitor := hcApplication.NewMonitorImp(splitsConfig, segmentsConfig, nil, logger)
	servicesMonitor := hcServices.NewMonitorImp(getServicesCountersConfig(*advanced), logger)

	// Creating Workers and Tasks
	telemetryRecorder := api.NewHTTPTelemetryRecorder(cfg.Apikey, *advanced, logger)
	telemetryConfigTask := pTasks.NewTelemetryConfigFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)
	telemetryUsageTask := pTasks.NewTelemetryUsageFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)
	telemetryKeysClientSideTask := pTasks.NewTelemetryKeysClientSideFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)
	telemetryKeysServerSideTask := pTasks.NewTelemetryKeysServerSideFlushTask(telemetryRecorder, logger, 1, tbufferSize, tworkers)

	// impression bulks & counts - events
	ibufferSize := int(cfg.Sync.Advanced.ImpressionsBuffer)
	iworkers := int(cfg.Sync.Advanced.ImpressionsWorkers)
	impressionRecorder := api.NewHTTPImpressionRecorder(cfg.Apikey, *advanced, logger)
	impressionTask := pTasks.NewImpressionsFlushTask(impressionRecorder, logger, 1, ibufferSize, iworkers)
	impressionCountTask := pTasks.NewImpressionCountFlushTask(impressionRecorder, logger, 1, ibufferSize, iworkers)
	eventsRecorder := api.NewHTTPEventsRecorder(cfg.Apikey, *advanced, logger)
	eventsTask := pTasks.NewEventsFlushTask(eventsRecorder, logger, 1, int(cfg.Sync.Advanced.EventsBuffer), int(cfg.Sync.Advanced.EventsWorkers))

	// setup feature flags, segments & local telemetry API interactions
	workers := synchronizer.Workers{
		SplitUpdater: caching.NewCacheAwareSplitSync(splitStorage, splitAPI.SplitFetcher, logger, localTelemetryStorage, httpCache, appMonitor, flagSetsFilter),
		SegmentUpdater: caching.NewCacheAwareSegmentSync(splitStorage, segmentStorage, splitAPI.SegmentFetcher, logger, localTelemetryStorage, httpCache,
			appMonitor),
		TelemetryRecorder: telemetry.NewTelemetrySynchronizer(localTelemetryStorage, telemetryRecorder, splitStorage, segmentStorage, logger,
			metadata, localTelemetryStorage),
	}

	// setup periodic tasks in case streaming is disabled or we need to fall back to polling
	stasks := synchronizer.SplitTasks{
		SplitSyncTask: tasks.NewFetchSplitsTask(workers.SplitUpdater, int(cfg.Sync.SplitRefreshRateMs/1000), logger),
		SegmentSyncTask: tasks.NewFetchSegmentsTask(workers.SegmentUpdater, int(cfg.Sync.SegmentRefreshRateMs/1000), advanced.SegmentWorkers,
			advanced.SegmentQueueSize, logger),
		TelemetrySyncTask:        tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, int(cfg.Sync.Advanced.InternalMetricsRateMs), logger),
		ImpressionSyncTask:       impressionTask,
		ImpressionsCountSyncTask: impressionCountTask,
		EventSyncTask:            eventsTask,
	}

	// Creating Synchronizer for tasks
	sync := ssync.NewSynchronizer(*advanced, stasks, workers, logger, nil, []tasks.Task{telemetryConfigTask, telemetryUsageTask, telemetryKeysClientSideTask, telemetryKeysServerSideTask})

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

	// Try to start bg sync in BG with unlimited retries (when a snapshot is provided),
	// the passed function is invoked upon initialization completion
	// If no snapshot is provided and init fails, `errUnrecoverable` is returned and application execution is aborted
	// health monitors are only started after successful init (otherwise they'll fail if the app doesn't sync correctly within the
	/// specified refresh period)
	before := time.Now()
	err = startBGSyng(syncManager, mstatus, cfg.Initialization.Snapshot != "", func() {
		logger.Info("Synchronizer tasks started")
		appMonitor.Start()
		servicesMonitor.Start()
		flagSetsAfterSanitize, _ := flagsets.SanitizeMany(cfg.FlagSetsFilter)
		workers.TelemetryRecorder.SynchronizeConfig(
			telemetry.InitConfig{
				AdvancedConfig: *advanced,
				TaskPeriods: conf.TaskPeriods{
					SplitSync:     int(cfg.Sync.SplitRefreshRateMs / 1000),
					SegmentSync:   int(cfg.Sync.SegmentRefreshRateMs / 1000),
					TelemetrySync: int(cfg.Sync.Advanced.InternalMetricsRateMs / 1000),
				},
				ListenerEnabled: cfg.Integrations.ImpressionListener.Endpoint != "",
				FlagSetsTotal:   int64(len(cfg.FlagSetsFilter)),
				FlagSetsInvalid: int64(len(cfg.FlagSetsFilter) - len(flagSetsAfterSanitize)),
			},
			time.Since(before).Milliseconds(),
			map[string]int64{cfg.Apikey: 1},
			nil,
		)
	})
	switch err {
	case errRetrying:
		logger.Warning("Failed to perform initial sync with Split servers but continuing from snapshot. Will keep retrying in BG")
	case errUnrecoverable:
		logger.Error("Initial synchronization failed. Either Split is unreachable or the SDK key is incorrect. Aborting execution.")
		return common.NewInitError(fmt.Errorf("error instantiating sync manager: %w", err), common.ExitTaskInitialization)
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

	adminTLSConfig, err := util.TLSConfigForServer(&cfg.Admin.TLS)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error setting up proxy TLS config: %w", err), common.ExitTLSError)
	}

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
		TLS:               adminTLSConfig,
	})
	if err != nil {
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}
	go adminServer.Start()

	tlsConfig, err := util.TLSConfigForServer(&cfg.Server.TLS)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error setting up proxy TLS config: %w", err), common.ExitTLSError)
	}

	proxyOptions := &Options{
		Logger:                      logger,
		Host:                        cfg.Server.Host,
		Port:                        int(cfg.Server.Port),
		APIKeys:                     cfg.Server.ClientApikeys,
		ImpressionListener:          nil,
		DebugOn:                     strings.ToLower(cfg.Logging.Level) == "debug" || strings.ToLower(cfg.Logging.Level) == "verbose",
		SplitFetcher:                splitAPI.SplitFetcher,
		ProxySplitStorage:           splitStorage,
		ProxySegmentStorage:         segmentStorage,
		ImpressionsSink:             impressionTask,
		ImpressionCountSink:         impressionCountTask,
		EventsSink:                  eventsTask,
		TelemetryConfigSink:         telemetryConfigTask,
		TelemetryUsageSink:          telemetryUsageTask,
		TelemetryKeysClientSideSink: telemetryKeysClientSideTask,
		TelemetryKeysServerSideSink: telemetryKeysServerSideTask,
		Telemetry:                   localTelemetryStorage,
		Cache:                       httpCache,
		TLSConfig:                   tlsConfig,
		FlagSets:                    cfg.FlagSetsFilter,
		FlagSetsStrictMatching:      cfg.FlagSetStrictMatching,
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

var (
	errRetrying      = errors.New("error but snapshot available")
	errUnrecoverable = errors.New("error and no snapshot available")
)

func startBGSyng(m synchronizer.Manager, mstatus chan int, haveSnapshot bool, onReady func()) error {

	attemptInit := func() bool {
		go m.Start()
		status := <-mstatus
		switch status {
		case synchronizer.Ready:
			onReady()
			return true
		case synchronizer.Error:
			return false
		}
		return false // should not reach here TODO:LOG!
	}

	if attemptInit() { // succeeeded at first try
		return nil
	}

	if !haveSnapshot {
		return errUnrecoverable
	}

	go func() {
		boff := backoff.New(2, 10*time.Minute)
		for !attemptInit() {
			time.Sleep(boff.Next())
		}
	}()

	return errRetrying

}

func getAppCounterConfigs() (hcAppCounter.ThresholdConfig, hcAppCounter.ThresholdConfig) {
	splitsConfig := hcAppCounter.DefaultThresholdConfig("Splits")
	segmentsConfig := hcAppCounter.DefaultThresholdConfig("Segments")

	return splitsConfig, segmentsConfig
}

func getServicesCountersConfig(advanced conf.AdvancedConfig) []hcServicesCounter.Config {
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
