package producer

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
	hcAppCounter "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application/counter"
	hcServicesCounter "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services/counter"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	config "github.com/splitio/go-split-commons/v10/conf"
	"github.com/splitio/go-split-commons/v10/provisional"
	"github.com/splitio/go-split-commons/v10/provisional/strategy"
	"github.com/splitio/go-split-commons/v10/service"
	storageCommon "github.com/splitio/go-split-commons/v10/storage"
	"github.com/splitio/go-split-commons/v10/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	impressionsCountPeriodTaskInMemory = 1800 // 30 min
	impressionObserverSize             = 500
)

func isValidApikey(splitFetcher service.SplitFetcher) bool {
	_, err := splitFetcher.Fetch(service.MakeFlagRequestParams().WithCacheControl(false).WithChangeNumber(time.Now().UnixNano() / int64(time.Millisecond)))
	return err == nil
}

func sanitizeRedis(cfg *conf.Main, miscStorage *redis.MiscStorage, logger logging.LoggerInterface) error {
	if miscStorage == nil {
		return errors.New("could not sanitize redis")
	}
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	currentHashAsStr := strconv.Itoa(int(currentHash))
	defer miscStorage.SetApikeyHash(currentHashAsStr)

	if cfg.Initialization.ForceFreshStartup {
		logger.Warning("Fresh startup requested. Cleaning up redis before initializing.")
		miscStorage.ClearAll()
		return nil
	}

	previousHashStr, err := miscStorage.GetApikeyHash()
	if err != nil && err.Error() != redis.ErrorHashNotPresent { // Missing hash is not considered an error
		return err
	}

	if currentHashAsStr != previousHashStr {
		logger.Warning("Previous SDK key is missing/different from current one. Cleaning up redis before startup.")
		miscStorage.ClearAll()
	}
	return nil
}

func getAppCounterConfigs(storage storageCommon.SplitStorage) (hcAppCounter.ThresholdConfig, hcAppCounter.ThresholdConfig, hcAppCounter.PeriodicConfig) {
	splitsConfig := hcAppCounter.DefaultThresholdConfig("Splits")
	segmentsConfig := hcAppCounter.DefaultThresholdConfig("Segments")
	storageConfig := hcAppCounter.PeriodicConfig{
		Name:                     "Storage",
		MaxErrorsAllowedInPeriod: 5,
		Period:                   3600,
		Severity:                 hcAppCounter.Low,
		ValidationFunc: func(c hcAppCounter.PeriodicCounterInterface) {
			_, err := storage.ChangeNumber()
			if err != nil {
				c.NotifyError()
			}
		},
		ValidationFuncPeriod: 10,
	}

	return splitsConfig, segmentsConfig, storageConfig
}

func getServicesCountersConfig(advanced *config.AdvancedConfig) []hcServicesCounter.Config {
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

func buildImpressionManager(
	impressionsMode string,
	impListener impressionlistener.ImpressionBulkListener,
	runtimeTelemetry storageCommon.TelemetryRuntimeProducer,
	impressionObserver strategy.ImpressionObserver,
	impressionsCounter *strategy.ImpressionsCounter,
) provisional.ImpressionManager {
	listenerEnabled := impListener != nil
	switch impressionsMode {
	case config.ImpressionsModeDebug:
		strategy := strategy.NewDebugImpl(impressionObserver, listenerEnabled)

		return provisional.NewImpressionManager(strategy)
	default:
		strategy := strategy.NewOptimizedImpl(impressionObserver, impressionsCounter, runtimeTelemetry, listenerEnabled)

		return provisional.NewImpressionManager(strategy)
	}
}
