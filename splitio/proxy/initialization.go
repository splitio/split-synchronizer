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
	if cfg.Storage.Redis.Enabled && redisClusterRequested(cfg) {
		return common.NewInitError(errors.New("redis state does not support Redis Cluster in this version; use a standalone or Sentinel Redis (clear the redis cluster nodes setting)"), common.ExitInvalidConfiguration)
	}

	if cfg.Initialization.OverridesFile != "" && !cfg.Offline {
		return common.NewInitError(errors.New("treatment overrides require offline mode in this version"), common.ExitInvalidConfiguration)
	}

	if err := cfg.Storage.Redis.Validate(); err != nil {
		return common.NewInitError(err, common.ExitInvalidConfiguration)
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
	// haveLocalState is true when the storages start with a complete snapshot (a Bolt snapshot file, or a Redis restore
	// with no skipped segment). The preload depends on it. The retry of a
	// failed first cloud sync uses firstSyncMayRetry, which also accepts a Redis restore with skipped segments.
	var haveLocalState = false
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

	// Boot order: a usable snapshot file wins and redis is not read; otherwise the redis boot read runs. With redis
	// enabled an unusable snapshot file is logged and falls through to the redis read, and is fatal only when redis
	// has no usable state either. An absent file is no snapshot when redis is enabled.
	snapFile := cfg.Initialization.Snapshot
	redisRead := bootAdminServer != nil
	var snapPayload []byte
	var snapErr error
	snapUsable := false
	if snapFile != "" {
		snapPayload, snapErr = loadBootSnapshot(cfg)
		snapUsable = snapErr == nil
		if redisRead && errors.Is(snapErr, snapshot.ErrNonexistantFile) {
			snapErr = nil
		}
	}
	if redisRead && snapErr != nil {
		logger.Warning(fmt.Sprintf("snapshot file is not usable, trying the redis state: %v", snapErr))
	}

	var boot BootDecision
	if snapUsable && redisRead {
		boot = BootDecision{Source: BootSourceSnapshotFile, Reason: "valid snapshot file"}
	} else {
		boot, err = resolveRedisBootInto(cfg, logger, newBootReadClient, sink)
		if err != nil {
			return err
		}
	}
	summary := boot.Summary()
	bootAdminServer.setSummary(summary)
	logBootSummary(logger, cfg, summary)
	if boot.Source != BootSourceRedis {
		discardBootDB()
	}
	bootAdminServer.setPhase(bootPhaseInitialSync)
	if boot.Source == BootSourceRedis {
		// Valid Redis state wins only over an unusable snapshot file; a usable file was taken before the redis read.
		haveLocalState = redisBootIsLocalState(boot)
	} else if snapFile != "" {
		if snapErr != nil {
			if !errors.Is(snapErr, errSnapshotConfigMismatch) || redisRead {
				return common.NewInitError(snapErr, common.ExitInvalidConfiguration)
			}
			logger.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one. Ignoring snapshot and starting with empty storage.")
		}
		if snapUsable {
			dbpath, err = snapshot.WritePayloadToTmpFile(snapPayload)
			if err != nil {
				return fmt.Errorf("error writing temporary snapshot file: %w", err)
			}
			haveLocalState = true
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
		preloadFromDisk := haveLocalState
		splitStorage, ruleBasedStorage, segmentStorage = newProxyStorages(dbInstance, logger, cfg, preloadFromDisk)
	}
	if err := applyRedisCursors(boot, splitStorage, ruleBasedStorage, segmentStorage); err != nil {
		return err
	}
	if afterRestoreHook != nil {
		afterRestoreHook(splitStorage)
	}
	// In lease mode the writer source stays closed until this process completes its first cloud sync, whatever the
	// boot source (D11). The writer acquires the lease only after the gate opens, so a stale snapshot or Bolt boot never
	// writes to Redis and never blocks a fresher proxy. With Redis disabled the gate is unused.
	redisGate := newSourceGate(false)
	redisWriter, err := setupRedisState(cfg, logger, splitStorage, ruleBasedStorage, segmentStorage, redisGate)
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
	err = startBGSync(syncManager, mstatus, firstSyncMayRetry(haveLocalState, boot), func() {
		liftRestoreTillCaps(splitStorage, ruleBasedStorage, segmentStorage)
		logger.Info("Synchronizer tasks started")
		// Opens the writer gate. In lease mode the writer's own Start resync runs against the populated source, because the
		// gate opens before any acquire, so no second resync is scheduled (D11).
		redisGate.open()
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
	maskAdminConfigSecrets(&cfgForAdmin)

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

// maskAdminConfigSecrets masks the credentials in the config served by the admin API: the Redis
// state connection credentials (username, password, TLS client key), the admin password, the Slack webhook
// (its URL carries a secret token) and the client apikeys. It mutates only the struct it is given, so callers pass a value copy of the
// config. The apikeys slice is replaced rather than edited, because the copy shares its backing
// array with the live config.
func maskAdminConfigSecrets(cfg *pconf.Main) {
	conn := &cfg.Storage.Redis.Connection
	for _, secret := range []*string{&conn.Username, &conn.Pass, &conn.TLSClientKey, &cfg.Storage.Redis.OfflineFingerprint, &cfg.Admin.Password, &cfg.Integrations.Slack.Webhook} {
		if *secret != "" {
			*secret = maskedSecret
		}
	}
	if cfg.Server.ClientApikeys != nil {
		masked := make([]string, len(cfg.Server.ClientApikeys))
		for i, key := range cfg.Server.ClientApikeys {
			masked[i] = key
			if key != "" {
				masked[i] = maskedSecret
			}
		}
		cfg.Server.ClientApikeys = masked
	}
}

// redisBootIsLocalState reports whether a boot decision gives the proxy a complete local state: a valid Redis restore
// with no skipped segment. It gates the preload (R8). A skipped segment is
// missing data, not a removal, so the writer must not treat it as complete. It does not decide the retry of a failed
// first cloud sync; firstSyncMayRetry does.
func redisBootIsLocalState(boot BootDecision) bool {
	return boot.Source == BootSourceRedis && boot.State != nil && len(boot.State.SkippedSegments) == 0
}

// errSnapshotConfigMismatch reports a snapshot made for another apikey, flag spec version or flag sets filter.
var errSnapshotConfigMismatch = errors.New("snapshot cfg (apikey, version, flagsets) does not match the provided one")

// loadBootSnapshot decodes and checks the configured snapshot file and returns its payload. It logs nothing. A missing
// file is returned wrapping snapshot.ErrNonexistantFile.
func loadBootSnapshot(cfg *pconf.Main) ([]byte, error) {
	snap, err := snapshot.DecodeFromFile(cfg.Initialization.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("error parsing snapshot file: %w", err)
	}
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	if snap.Meta().Hash != strconv.Itoa(int(currentHash)) {
		return nil, errSnapshotConfigMismatch
	}
	payload, err := snap.Data()
	if err != nil {
		return nil, fmt.Errorf("error reading snapshot payload: %w", err)
	}
	if err := checkSnapshot(snap.Meta(), payload, cfg.FlagSpecVersion, false); err != nil {
		return nil, err
	}
	return payload, nil
}

// firstSyncMayRetry reports whether a failed first cloud sync is retried in the background instead of exiting. It is
// true for any local state, and also for a valid Redis restore with skipped segments: the first sync fetches the
// skipped segments, so the proxy can serve the restored state while it retries. It is deliberately not
// redisBootIsLocalState, because the preload needs a complete state.
func firstSyncMayRetry(haveLocalState bool, boot BootDecision) bool {
	return haveLocalState || (boot.Source == BootSourceRedis && boot.State != nil)
}

// Cold start (no local state and no Redis restore) retries the first sync a bounded number of times (D7).
const (
	coldStartMaxAttempts = 5
	coldStartBackoffBase = time.Second
	coldStartBackoffCap  = 30 * time.Second
)

// coldStartSleep waits between cold start attempts. Tests replace it to avoid real sleeps.
var coldStartSleep = time.Sleep

// coldStartBackoff returns the wait before retry number attempt (1-based): base doubled per attempt, capped.
func coldStartBackoff(attempt int) time.Duration {
	d := coldStartBackoffBase << (attempt - 1)
	if d > coldStartBackoffCap {
		return coldStartBackoffCap
	}
	return d
}

func startBGSync(m synchronizer.Manager, mstatus chan int, haveLocalState bool, onReady func()) error {
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

	if !haveLocalState {
		// A failed sync manager resets its lifecycle, so the same manager can start again.
		for attempt := 1; attempt < coldStartMaxAttempts; attempt++ {
			coldStartSleep(coldStartBackoff(attempt))
			if attemptInit() {
				return nil
			}
		}
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
