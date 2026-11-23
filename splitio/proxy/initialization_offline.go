package proxy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/admin"
	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	pTasks "github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	commonsflagsets "github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-split-commons/v10/service/api/specs"
	inmemory "github.com/splitio/go-split-commons/v10/storage/inmemory/mutexmap"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

// loadOfflineSnapshot validates offline configuration and returns the decoded snapshot with its uncompressed payload.
func loadOfflineSnapshot(cfg *pconf.Main) (*snapshot.Snapshot, []byte, error) {
	if err := checkOfflineIdentity(cfg); err != nil {
		return nil, nil, err
	}
	if cfg.Initialization.Snapshot == "" {
		return nil, nil, common.NewInitError(errors.New("offline mode requires a snapshot"), common.ExitInvalidConfiguration)
	}

	snap, err := snapshot.DecodeFromFile(cfg.Initialization.Snapshot)
	if err != nil {
		return nil, nil, common.NewInitError(fmt.Errorf("error parsing snapshot file: %w", err), common.ExitInvalidConfiguration)
	}
	payload, err := snap.Data()
	if err != nil {
		return nil, nil, common.NewInitError(fmt.Errorf("error reading snapshot payload: %w", err), common.ExitInvalidConfiguration)
	}
	if err := checkSnapshot(snap.Meta(), payload, cfg.FlagSpecVersion, true); err != nil {
		return nil, nil, common.NewInitError(err, common.ExitInvalidConfiguration)
	}
	return snap, payload, nil
}

// checkOfflineIdentity rejects an API key and an unsupported flag spec version. Offline mode has no API key.
func checkOfflineIdentity(cfg *pconf.Main) error {
	if strings.TrimSpace(cfg.Apikey) != "" {
		return common.NewInitError(errors.New("SPLIT_PROXY_APIKEY must be empty when offline mode is enabled"), common.ExitInvalidConfiguration)
	}
	if specs.Match(cfg.FlagSpecVersion) == nil {
		return common.NewInitError(fmt.Errorf("flag spec version %q is not supported", cfg.FlagSpecVersion), common.ExitInvalidConfiguration)
	}
	return nil
}

// offlineBoot is the state an offline proxy serves and the source it was read from.
type offlineBoot struct {
	source           BootSource
	snapSpec         string // flag spec version recorded in the snapshot, or the configured one for a redis boot
	dbInstance       *persistent.BoltDBWrapper
	splitStorage     *storage.ProxySplitStorageImpl
	ruleBasedStorage *storage.ProxyRuleBasedSegmentsStorageImpl
	segmentStorage   *storage.ProxySegmentStorageImpl
}

// bootOffline picks the state an offline proxy serves. A usable snapshot file wins. With redis enabled an absent or
// unusable snapshot falls back to the redis boot read, which only reads and never writes or takes the lease. It fails
// with ExitInvalidConfiguration when neither source is usable.
func bootOffline(logger logging.LoggerInterface, cfg *pconf.Main) (*offlineBoot, error) {
	if err := checkOfflineIdentity(cfg); err != nil {
		return nil, err
	}
	if !cfg.Storage.Redis.Enabled {
		return bootOfflineSnapshot(logger, cfg)
	}
	if err := validateOfflineRedisSettings(cfg); err != nil {
		return nil, err
	}
	var snapErr error
	if cfg.Initialization.Snapshot != "" {
		boot, err := bootOfflineSnapshot(logger, cfg)
		if err == nil {
			return boot, nil
		}
		snapErr = err
		logger.Warning(fmt.Sprintf("snapshot file is not usable, trying the redis state: %v", err))
	}
	boot, err := bootOfflineRedis(logger, cfg)
	if err != nil {
		if snapErr != nil {
			return nil, common.NewInitError(fmt.Errorf("no usable offline state: snapshot file: %v; redis state: %v", snapErr, err), common.ExitInvalidConfiguration)
		}
		return nil, err
	}
	return boot, nil
}

// bootOfflineSnapshot serves the snapshot file through a Bolt database built from its payload.
func bootOfflineSnapshot(logger logging.LoggerInterface, cfg *pconf.Main) (*offlineBoot, error) {
	snap, payload, err := loadOfflineSnapshot(cfg)
	if err != nil {
		return nil, err
	}
	logger.Info("Split Proxy offline mode: serving snapshot ", cfg.Initialization.Snapshot)

	dbpath, err := snapshot.WritePayloadToTmpFile(payload)
	if err != nil {
		return nil, fmt.Errorf("error writing temporary snapshot file: %w", err)
	}
	logger.Debug("Database created from snapshot at", dbpath)

	dbInstance, err := persistent.NewBoltWrapper(dbpath, nil)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}

	// Flag-set filtering of a loaded snapshot is a follow-up. Serve the file as stored.
	emptyFilter := commonsflagsets.NewFlagSetFilter(nil)
	return &offlineBoot{
		source:           BootSourceSnapshotFile,
		snapSpec:         snap.Meta().FlagSpecVersion,
		dbInstance:       dbInstance,
		splitStorage:     storage.NewProxySplitStorage(dbInstance, logger, emptyFilter, true),
		ruleBasedStorage: storage.NewProxyRuleBasedSegmentsStorage(dbInstance, logger, true),
		segmentStorage:   storage.NewProxySegmentStorage(dbInstance, logger, true),
	}, nil
}

// bootOfflineRedis restores the redis state into an in-memory database with the online boot read. The read client
// is opened and closed by resolveRedisBootInto and issues read commands only: offline starts no writer, takes no lease
// and writes no key (INV-8). The state must be valid and match the configured offline fingerprint, otherwise it fails.
func bootOfflineRedis(logger logging.LoggerInterface, cfg *pconf.Main) (*offlineBoot, error) {
	dbInstance, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}
	emptyFilter := commonsflagsets.NewFlagSetFilter(nil)
	splitStorage := storage.NewProxySplitStorage(dbInstance, logger, emptyFilter, false)
	ruleBasedStorage := storage.NewProxyRuleBasedSegmentsStorage(dbInstance, logger, false)
	segmentStorage := storage.NewProxySegmentStorage(dbInstance, logger, false)
	sink := storage.NewRedisRestoreSink(splitStorage, ruleBasedStorage, segmentStorage)

	boot, err := resolveRedisBootInto(cfg, logger, newBootReadClient, sink)
	if err == nil && boot.Source != BootSourceRedis {
		err = common.NewInitError(fmt.Errorf("redis state is not usable offline: %s", boot.Reason), common.ExitInvalidConfiguration)
	}
	if err == nil {
		err = applyRedisCursors(boot, splitStorage, ruleBasedStorage, segmentStorage)
	}
	if err != nil {
		if derr := dbInstance.Discard(); derr != nil {
			logger.Warning(fmt.Sprintf("error discarding the temporary boot database: %v", derr))
		}
		return nil, err
	}
	logger.Info("Split Proxy offline mode: serving the redis state")
	return &offlineBoot{
		source:           BootSourceRedis,
		snapSpec:         cfg.FlagSpecVersion,
		dbInstance:       dbInstance,
		splitStorage:     splitStorage,
		ruleBasedStorage: ruleBasedStorage,
		segmentStorage:   segmentStorage,
	}, nil
}

// warnOfflineFlagSetsFilter tells the operator that a configured flag sets filter has no effect offline.
func warnOfflineFlagSetsFilter(logger logging.LoggerInterface, filter []string) {
	if len(filter) == 0 {
		return
	}
	logger.Warning("Split Proxy offline mode: the flag sets filter [" + strings.Join(filter, ",") + "] is ignored. The snapshot is served as stored.")
}

// warnOfflineLargeSegments tells the operator that flags referencing large segments will return the default treatment,
// because large-segment keys are never stored in a snapshot.
func warnOfflineLargeSegments(logger logging.LoggerInterface, names *set.ThreadUnsafeSet) {
	if names == nil || names.IsEmpty() {
		return
	}
	items := make([]string, 0, names.Size())
	for _, name := range names.List() {
		items = append(items, fmt.Sprint(name))
	}
	logger.Warning("Split Proxy offline mode: the snapshot references large segments [" + strings.Join(items, ",") + "] but large-segment keys are not stored in snapshots. Flags that use them return the default treatment.")
}

func startOffline(logger logging.LoggerInterface, cfg *pconf.Main) error {
	boot, err := bootOffline(logger, cfg)
	if err != nil {
		return err
	}
	dbInstance := boot.dbInstance
	splitStorage := boot.splitStorage
	ruleBasedStorage := boot.ruleBasedStorage
	segmentStorage := boot.segmentStorage

	cfg.Sync.Advanced.StreamingEnabled = false
	warnOfflineFlagSetsFilter(logger, cfg.FlagSetsFilter)
	warnOfflineLargeSegments(logger, splitStorage.LargeSegmentNames())
	largeSegmentStorage := inmemory.NewLargeSegmentsStorage()
	localTelemetryStorage := storage.NewTimeslicedProxyEndpointTelemetry(
		storage.NewProxyTelemetryFacade(),
		cfg.Observability.TimeSliceWidthSecs,
		int(cfg.Observability.MaxTimeSliceCount),
	)
	httpCache := caching.MakeProxyCache()

	// Load and validate before anything starts, so a bad overrides file exits with nothing running.
	flagOverrides, overridesStamp, err := setupOverrides(logger, cfg.Initialization.OverridesFile, splitStorage, time.Now)
	if err != nil {
		return err
	}

	appMonitor := newStaticAppMonitor()
	servicesMonitor := newStaticServicesMonitor()
	appMonitor.Start()
	servicesMonitor.Start()

	drop := pTasks.NewNoopRecordingTask()
	rtm := common.NewRuntime(false, idleSyncManager{}, logger, "Split Proxy", nil, nil, appMonitor, servicesMonitor)
	storages := adminCommon.Storages{
		SplitStorage:             splitStorage,
		SegmentStorage:           segmentStorage,
		LocalTelemetryStorage:    localTelemetryStorage,
		LargeSegmentStorage:      largeSegmentStorage,
		RuleBasedSegmentsStorage: ruleBasedStorage,
	}

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
		SnapshotFlagSpec:  boot.snapSpec,
		FlagSetsFilter:    cfg.FlagSetsFilter,
		Hash:              strconv.Itoa(int(hash)),
	})
	if err != nil {
		return common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}
	go adminServer.Start()

	tlsConfig, err := util.TLSConfigForServer(&cfg.Server.TLS)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error setting up proxy TLS config: %w", err), common.ExitTLSError)
	}

	proxyAPI := New(&Options{
		Logger:                      logger,
		Host:                        cfg.Server.Host,
		Port:                        int(cfg.Server.Port),
		APIKeys:                     cfg.Server.ClientApikeys,
		DebugOn:                     strings.ToLower(cfg.Logging.Level) == "debug" || strings.ToLower(cfg.Logging.Level) == "verbose",
		SplitFetcher:                offlineSplitFetcher{},
		ProxySplitStorage:           splitStorage,
		ProxySegmentStorage:         segmentStorage,
		ProxyRBSegmentStorage:       ruleBasedStorage,
		ImpressionsSink:             drop,
		ImpressionCountSink:         drop,
		EventsSink:                  drop,
		TelemetryConfigSink:         drop,
		TelemetryUsageSink:          drop,
		TelemetryKeysClientSideSink: drop,
		TelemetryKeysServerSideSink: drop,
		Telemetry:                   localTelemetryStorage,
		Cache:                       httpCache,
		TLSConfig:                   tlsConfig,
		FlagSets:                    cfg.FlagSetsFilter,
		FlagSetsStrictMatching:      cfg.FlagSetStrictMatching,
		ProxyLargeSegmentStorage:    largeSegmentStorage,
		SpecVersion:                 cfg.FlagSpecVersion,
		Offline:                     true,
		Overrides:                   flagOverrides,
		OverridesStamp:              overridesStamp,
	})
	go proxyAPI.Start()

	rtm.RegisterShutdownHandler()
	rtm.Block()
	return nil
}
