package proxy

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	pTasks "github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	"github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/engine/grammar"
	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-split-commons/v10/service/api"
	inmemory "github.com/splitio/go-split-commons/v10/storage/inmemory/mutexmap"
	"github.com/splitio/go-split-commons/v10/synchronizer"
	"github.com/splitio/go-split-commons/v10/tasks"
	"github.com/splitio/go-split-commons/v10/telemetry"
	"github.com/splitio/go-toolkit/v5/backoff"
	"github.com/splitio/go-toolkit/v5/logging"
)

// Start initialize in proxy mode
func Start(logger logging.LoggerInterface, cfg *pconf.Main) error {
	return start(logger, cfg)
}

// afterRestoreHook is a test seam, nil in production. It is called after the Redis cursors are applied and before
// the Redis state writer is set up.
var afterRestoreHook func(*storage.ProxySplitStorageImpl)

// redisClusterRequested mirrors the choice made by common.ParseRedisOptions: Sentinel wins over cluster mode, and the
// cluster nodes setting is ignored unless cluster mode is on.
func redisClusterRequested(cfg *pconf.Main) bool {
	conn := cfg.Storage.Redis.Connection
	return conn.ClusterMode && !conn.SentinelReplication
}

func start(logger logging.LoggerInterface, cfg *pconf.Main) error {
	if cfg.Offline && cfg.Storage.Redis.Enabled {
		return common.NewInitError(errors.New("redis state is not supported in offline mode"), common.ExitInvalidConfiguration)
	}

	if cfg.Storage.Redis.Enabled && redisClusterRequested(cfg) {
		return common.NewInitError(errors.New("redis state does not support Redis Cluster in this version; use a standalone or Sentinel Redis (clear the redis cluster nodes setting)"), common.ExitInvalidConfiguration)
	}

	if cfg.Initialization.OverridesFile != "" && !cfg.Offline {
		return common.NewInitError(errors.New("treatment overrides require offline mode in this version"), common.ExitInvalidConfiguration)
	}

	if cfg.Offline {
		return startOffline(logger, cfg)
	}

	clientKey, err := util.GetClientKey(cfg.Apikey)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error parsing client key from provided apikey: %w", err), common.ExitInvalidApikey)
	}

	// Initialization of DB
	var dbpath = persistent.BoltInMemoryMode
	var snapshotValid = false
	var bootAdminServer *bootAdmin
	if cfg.Storage.Redis.Enabled && !cfg.Initialization.ForceFreshStartup {
		// Validate before binding so a bad setting is reported without a port conflict masking it. The check runs
		// again inside resolveRedisBootInto on purpose: it is cheap and that path is also exercised directly.
		if err = validateRedisStateSettings(&cfg.Storage.Redis); err != nil {
			return err
		}
		// Hold the admin address from before the redis read until the full admin server takes over.
		bootAdminServer, err = startBootAdmin(cfg, logger)
		if err != nil {
			return err
		}
		defer bootAdminServer.stop()
	}
	// With redis enabled the in-memory db and the storages exist before the read, so the reader restores each segment
	// into them as it goes instead of holding every segment in memory. They are thrown away unless redis wins.
	var dbInstance *persistent.BoltDBWrapper
	var splitStorage *storage.ProxySplitStorageImpl
	var ruleBasedStorage *storage.ProxyRuleBasedSegmentsStorageImpl
	var segmentStorage *storage.ProxySegmentStorageImpl
	var sink redisstate.RestoreSink
	if bootAdminServer != nil {
		dbInstance, err = persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
		}
		splitStorage, ruleBasedStorage, segmentStorage = newProxyStorages(dbInstance, logger, cfg, false)
		sink = storage.NewRedisRestoreSink(splitStorage, ruleBasedStorage, segmentStorage)
	}
	discardBootDB := func() {
		if dbInstance == nil {
			return
		}
		if derr := dbInstance.Discard(); derr != nil {
			logger.Warning(fmt.Sprintf("error discarding the temporary boot database: %v", derr))
		}
		dbInstance, splitStorage, ruleBasedStorage, segmentStorage = nil, nil, nil, nil
	}
	keepBootDB := false
	defer func() {
		if !keepBootDB {
			discardBootDB()
		}
	}()

	boot, err := resolveRedisBootInto(cfg, logger, newBootReadClient, sink)
	if err != nil {
		return err
	}
	if boot.Source != BootSourceRedis {
		discardBootDB()
	}
	bootAdminServer.setPhase(bootPhaseInitialSync)
	if boot.Source == BootSourceRedis {
		// Valid Redis state wins; the snapshot file is not parsed, so a bad file cannot abort the boot.
		// A state with skipped segments is incomplete, so it does not excuse a failed first cloud sync.
		snapshotValid = len(boot.State.SkippedSegments) == 0
	}
	if snapFile := cfg.Initialization.Snapshot; snapFile != "" && boot.Source != BootSourceRedis {
		snap, err := snapshot.DecodeFromFile(snapFile)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error parsing snapshot file: %w", err), common.ExitInvalidConfiguration)
		}

		currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
		if snap.Meta().Hash != strconv.Itoa(int(currentHash)) {
			logger.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one. Ignoring snapshot and starting with empty storage.")
		} else {
			payload, err := snap.Data()
			if err != nil {
				return common.NewInitError(fmt.Errorf("error reading snapshot payload: %w", err), common.ExitInvalidConfiguration)
			}
			if err := checkSnapshot(snap.Meta(), payload, cfg.FlagSpecVersion, false); err != nil {
				return common.NewInitError(err, common.ExitInvalidConfiguration)
			}
			// Hash matches - use the snapshot
			dbpath, err = snapshot.WritePayloadToTmpFile(payload)
			if err != nil {
				return fmt.Errorf("error writing temporary snapshot file: %w", err)
			}
			snapshotValid = true
			logger.Debug("Database created from snapshot at", dbpath)
		}
	}

	if dbInstance == nil {
		dbInstance, err = persistent.NewBoltWrapper(dbpath, nil)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
		}
	}

	// Set up the http proxy caching.
	// We need it fairly early since it's passed to the synchronizers, so that they can evict entries when a change is processed
	httpCache := caching.MakeProxyCache()

	// Getting initial config data
	advanced := cfg.BuildAdvancedConfig()
	advanced.FlagSetsFilter = cfg.FlagSetsFilter
	advanced.AuthSpecVersion = cfg.FlagSpecVersion
	advanced.FlagsSpecVersion = cfg.FlagSpecVersion
	common.LogStreamingForceHTTP1(logger, advanced.StreamingEnabled, advanced.StreamingForceHTTP1)
	metadata := util.GetMetadata(cfg.IPAddressEnabled, true)

	// FlagSetsFilter
	flagSetsFilter := flagsets.NewFlagSetFilter(cfg.FlagSetsFilter)

	// Setup fetchers & recorders
	splitAPI := api.NewSplitAPI(cfg.Apikey, *advanced, logger, metadata)

	// Proxy storages already implement the observable interface, so no need to wrap them
	// Redis boots set splitStorage earlier (sink setup) and restore into it, so this branch runs only for non-Redis boots.
	if splitStorage == nil {
		preloadFromDisk := snapshotValid
		splitStorage, ruleBasedStorage, segmentStorage = newProxyStorages(dbInstance, logger, cfg, preloadFromDisk)
	}
	if err := applyRedisCursors(boot, splitStorage, ruleBasedStorage, segmentStorage); err != nil {
		return err
	}
	if afterRestoreHook != nil {
		afterRestoreHook(splitStorage)
	}
	redisWriter, err := setupRedisState(cfg, logger, splitStorage, ruleBasedStorage, segmentStorage)
	if err != nil {
		return err
	}
	if redisWriter != nil {
		defer stopRedisState(redisWriter, logger, stopWaitBound(cfg.Storage.Redis, cfg.Storage.Redis.Connection))
	}
	largeSegmentStorage := inmemory.NewLargeSegmentsStorage()

	// Local telemetry
	tbufferSize := int(cfg.Sync.Advanced.TelemetryBuffer)
	tworkers := int(cfg.Sync.Advanced.TelemetryWorkers)

	localTelemetryStorage := storage.NewTimeslicedProxyEndpointTelemetry(
		storage.NewProxyTelemetryFacade(),
		cfg.Observability.TimeSliceWidthSecs,
		int(cfg.Observability.MaxTimeSliceCount),
	)

	// Healcheck Monitor
	splitsConfig, segmentsConfig, lsConfig := getAppCounterConfigs()
	appMonitor := hcApplication.NewMonitorImp(splitsConfig, segmentsConfig, &lsConfig, nil, logger)
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

	ruleBuilder := grammar.NewRuleBuilder(
		segmentStorage,
		ruleBasedStorage,
		largeSegmentStorage,
		adminCommon.ProxyFeatureFlagsRules,
		adminCommon.ProducerRuleBasedSegmentRules,
		logger,
		nil)

	// setup feature flags, segments & local telemetry API interactions
	workers := synchronizer.Workers{
		SplitUpdater: caching.NewCacheAwareSplitSync(splitStorage, ruleBasedStorage, splitAPI.SplitFetcher, logger, localTelemetryStorage, httpCache, appMonitor, flagSetsFilter, advanced.FlagsSpecVersion, ruleBuilder),
		SegmentUpdater: caching.NewCacheAwareSegmentSync(splitStorage, segmentStorage, ruleBasedStorage, splitAPI.SegmentFetcher, logger, localTelemetryStorage, httpCache,
			appMonitor),
		TelemetryRecorder: telemetry.NewTelemetrySynchronizer(localTelemetryStorage, telemetryRecorder, splitStorage, segmentStorage, logger,
			metadata, localTelemetryStorage),
		LargeSegmentUpdater: caching.NewCacheAwareLargeSegmentSync(splitStorage, largeSegmentStorage, splitAPI.LargeSegmentFetcher, logger, localTelemetryStorage, httpCache, appMonitor),
	}

	// setup periodic tasks in case streaming is disabled or we need to fall back to polling
	stasks := synchronizer.SplitTasks{
		SplitSyncTask: tasks.NewFetchSplitsTask(workers.SplitUpdater, int(cfg.Sync.SplitRefreshRateMs/1000), logger),
		SegmentSyncTask: tasks.NewFetchSegmentsTask(workers.SegmentUpdater, int(cfg.Sync.SegmentRefreshRateMs/1000), advanced.SegmentWorkers,
			advanced.SegmentQueueSize, logger, appMonitor),
		TelemetrySyncTask:        tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, int(cfg.Sync.Advanced.InternalMetricsRateMs), logger),
		ImpressionSyncTask:       impressionTask,
		ImpressionsCountSyncTask: impressionCountTask,
		EventSyncTask:            eventsTask,
		LargeSegmentSyncTask:     tasks.NewFetchLargeSegmentsTask(workers.LargeSegmentUpdater, splitStorage, advanced.LargeSegment.RefreshRate, advanced.LargeSegment.Workers, advanced.LargeSegment.QueueSize, logger, appMonitor),
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
	// An error return after the sync tasks start must stop them before the deferred boot db discard runs (defers run
	// last in first out), otherwise they keep writing into a closed db. Stop is a no-op if the manager never ran.
	defer func() {
		if !keepBootDB {
			syncManager.Stop()
		}
	}()

	// Try to start bg sync in BG with unlimited retries (when a valid snapshot is provided),
	// the passed function is invoked upon initialization completion
	// If no valid snapshot is provided and init fails, `errUnrecoverable` is returned and application execution is aborted
	// health monitors are only started after successful init (otherwise they'll fail if the app doesn't sync correctly within the
	/// specified refresh period)
	before := time.Now()
	err = startBGSync(syncManager, mstatus, snapshotValid, func() {
		logger.Info("Synchronizer tasks started")
		// The writer's startup resync can run against an empty source on a cold boot, so repair it once sync
		// has populated the storages.
		requestRedisResync(redisWriter)
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
		SplitStorage:             splitStorage,
		SegmentStorage:           segmentStorage,
		LocalTelemetryStorage:    localTelemetryStorage,
		LargeSegmentStorage:      largeSegmentStorage,
		RuleBasedSegmentsStorage: ruleBasedStorage,
	}
	if redisWriter != nil {
		storages.RedisState = redisWriter
	}

	// --------------------------- ADMIN DASHBOARD ------------------------------
	cfgForAdmin := *cfg
	hash := util.HashAPIKey(cfgForAdmin.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	cfgForAdmin.Apikey = logging.ObfuscateAPIKey(cfgForAdmin.Apikey)
	maskRedisStateSecrets(&cfgForAdmin)

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
		FlagSpecVersion:   cfg.FlagSpecVersion,
		FlagSetsFilter:    cfg.FlagSetsFilter,
		Hash:              strconv.Itoa(int(hash)),
	})
	if err != nil {
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}
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
		ProxyRBSegmentStorage:       ruleBasedStorage,
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
		ProxyLargeSegmentStorage:    largeSegmentStorage,
		SpecVersion:                 cfg.FlagSpecVersion,
	}

	if ilcfg := cfg.Integrations.ImpressionListener; ilcfg.Endpoint != "" {
		var err error
		proxyOptions.ImpressionListener, err = impressionlistener.NewImpressionBulkListener(ilcfg.Endpoint, int(ilcfg.QueueSize), nil)
		if err != nil {
			return common.NewInitError(fmt.Errorf("error instantiating impression listener: %w", err), common.ExitTaskInitialization)
		}
		proxyOptions.ImpressionListener.Start()
	}

	bootAdminServer.stop()
	go func() {
		if err := adminServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(fmt.Sprintf("admin server stopped: %v", err))
		}
	}()

	proxyAPI := New(proxyOptions)
	go proxyAPI.Start()

	rtm.RegisterShutdownHandler()
	// Past this point the process serves until shutdown, so the restored boot DB is kept.
	keepBootDB = true
	rtm.Block()
	return nil
}

var (
	errRetrying      = errors.New("error but snapshot available")
	errUnrecoverable = errors.New("error and no snapshot available")
)

// maskedSecret replaces non-empty credentials in the config served by the admin API.
const maskedSecret = "********"

// maskRedisStateSecrets masks the Redis state connection credentials (username, password,
// TLS client key). It mutates only the struct it is given, so callers pass a value copy
// of the config and the live config keeps its secrets.
func maskRedisStateSecrets(cfg *pconf.Main) {
	conn := &cfg.Storage.Redis.Connection
	for _, secret := range []*string{&conn.Username, &conn.Pass, &conn.TLSClientKey} {
		if *secret != "" {
			*secret = maskedSecret
		}
	}
}

// requestRedisResync asks a non-nil Redis writer for a full resync. A nil writer (feature disabled) is a no-op.
func requestRedisResync(w *redisstate.Writer) {
	if w != nil {
		w.RequestResync()
	}
}

func startBGSync(m synchronizer.Manager, mstatus chan int, haveSnapshot bool, onReady func()) error {
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

func getAppCounterConfigs() (hcAppCounter.ThresholdConfig, hcAppCounter.ThresholdConfig, hcAppCounter.ThresholdConfig) {
	splitsConfig := hcAppCounter.DefaultThresholdConfig("Splits")
	segmentsConfig := hcAppCounter.DefaultThresholdConfig("Segments")
	LargeSegmentsConfig := hcAppCounter.DefaultThresholdConfig("LargeSegments")

	return splitsConfig, segmentsConfig, LargeSegmentsConfig
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
