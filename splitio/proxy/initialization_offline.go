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
	"github.com/splitio/go-toolkit/v5/logging"
)

// loadOfflineSnapshot validates offline configuration and returns the decoded snapshot with its uncompressed payload.
func loadOfflineSnapshot(cfg *pconf.Main) (*snapshot.Snapshot, []byte, error) {
	if strings.TrimSpace(cfg.Apikey) != "" {
		return nil, nil, common.NewInitError(errors.New("SPLIT_PROXY_APIKEY must be empty when offline mode is enabled"), common.ExitInvalidConfiguration)
	}
	if cfg.Initialization.Snapshot == "" {
		return nil, nil, common.NewInitError(errors.New("offline mode requires a snapshot"), common.ExitInvalidConfiguration)
	}
	if specs.Match(cfg.FlagSpecVersion) == nil {
		return nil, nil, common.NewInitError(fmt.Errorf("flag spec version %q is not supported", cfg.FlagSpecVersion), common.ExitInvalidConfiguration)
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

func startOffline(logger logging.LoggerInterface, cfg *pconf.Main) error {
	snap, payload, err := loadOfflineSnapshot(cfg)
	if err != nil {
		return err
	}

	cfg.Sync.Advanced.StreamingEnabled = false
	logger.Info("Split Proxy offline mode: serving snapshot ", cfg.Initialization.Snapshot)

	dbpath, err := snapshot.WritePayloadToTmpFile(payload)
	if err != nil {
		return fmt.Errorf("error writing temporary snapshot file: %w", err)
	}
	logger.Debug("Database created from snapshot at", dbpath)

	dbInstance, err := persistent.NewBoltWrapper(dbpath, nil)
	if err != nil {
		return common.NewInitError(fmt.Errorf("error instantiating boltdb: %w", err), common.ExitErrorDB)
	}

	// Flag-set filtering of a loaded snapshot is a follow-up. Serve the file as stored.
	emptyFilter := commonsflagsets.NewFlagSetFilter(nil)
	splitStorage := storage.NewProxySplitStorage(dbInstance, logger, emptyFilter, true)
	ruleBasedStorage := storage.NewProxyRuleBasedSegmentsStorage(dbInstance, logger, true)
	segmentStorage := storage.NewProxySegmentStorage(dbInstance, logger, true)
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
		SnapshotFlagSpec:  snap.Meta().FlagSpecVersion,
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
