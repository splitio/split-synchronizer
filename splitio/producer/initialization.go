package producer

import (
	"errors"
	"fmt"
	l "log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	config "github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	predis "github.com/splitio/go-split-commons/storage/redis"
	"github.com/splitio/go-split-commons/synchronizer"
	"github.com/splitio/go-split-commons/synchronizer/worker"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/go-toolkit/nethelpers"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/common"
	multipleWorkers "github.com/splitio/split-synchronizer/splitio/producer/worker"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/util"
	"github.com/splitio/split-synchronizer/splitio/web/admin"
)

func gracefulShutdownProducer(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager *synchronizer.Manager) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	syncManager.Stop()

	// Healthcheck - Emit task stop signal
	fmt.Println(" -> Sending STOP to healthcheck goroutine")
	task.StopHealtcheck()

	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

func startLoop(loopTime int64) {
	for {
		time.Sleep(time.Duration(loopTime) * time.Millisecond)
	}
}

func hashAPIKey(apikey string) uint32 {
	return util.Murmur3_32([]byte(apikey), 0)
}

func sanitizeRedis(miscStorage *predis.MiscStorage, logger logging.LoggerInterface) error {
	if miscStorage == nil {
		return errors.New("Could not sanitize redis")
	}
	currentHash := hashAPIKey(conf.Data.APIKey)
	currentHashAsStr := strconv.Itoa(int(currentHash))
	defer miscStorage.SetApikeyHash(currentHashAsStr)

	if conf.Data.Redis.ForceFreshStartup {
		logger.Warning("Fresh startup requested. Cleaning up redis before initializing.")
		miscStorage.ClearAll()
		return nil
	}

	previousHashStr, err := miscStorage.GetApikeyHash()
	if err != nil && err.Error() != predis.ErrorHashNotPresent { // Missing hash is not considered an error
		return err
	}

	if currentHashAsStr != previousHashStr {
		logger.Warning("Previous apikey is missing/different from current one. Cleaning up redis before startup.")
		miscStorage.ClearAll()
	}
	return nil
}

func isValidApikey(splitFetcher service.SplitFetcher) bool {
	_, err := splitFetcher.Fetch(time.Now().UnixNano() / int64(time.Millisecond))
	return err == nil
}

// Start initialize the producer mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	logger := logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: l.LUTC | l.Ldate | l.Lmicroseconds | l.Lshortfile,
		LogLevel:            logging.LevelInfo,
	})

	conf.Initialize()
	advanced := config.GetDefaultAdvancedConfig()
	advanced.EventsBulkSize = conf.Data.EventsPerPost
	advanced.HTTPTimeout = int(conf.Data.HTTPTimeout)
	advanced.ImpressionsBulkSize = conf.Data.ImpressionsPerPost
	// EventsQueueSize:      5000, // MISSING
	// ImpressionsQueueSize: 5000, // MISSING
	// SegmentQueueSize:     100,  // MISSING
	// SegmentWorkers:       10,   // MISSING

	envSdkURL := os.Getenv("SPLITIO_SDK_URL")
	if envSdkURL != "" {
		advanced.SdkURL = envSdkURL
	} else {
		advanced.SdkURL = "https://sdk.split.io/api"
	}

	envEventsURL := os.Getenv("SPLITIO_EVENTS_URL")
	if envEventsURL != "" {
		advanced.EventsURL = envEventsURL
	} else {
		advanced.EventsURL = "https://events.split.io/api"
	}

	instanceName := "unknown"
	ipAddress := "unknown"
	if conf.Data.IPAddressesEnabled {
		ip, err := nethelpers.ExternalIP()
		if err == nil {
			ipAddress = ip
			instanceName = fmt.Sprintf("ip-%s", strings.Replace(ipAddress, ".", "-", -1))
		}
	}

	metadata := dtos.Metadata{
		MachineIP:   ipAddress,
		MachineName: instanceName,
		SDKVersion:  "split-sync-" + splitio.Version,
	}

	// Setup fetchers & recorders
	splitAPI := service.NewSplitAPI(
		conf.Data.APIKey,
		advanced,
		logger,
		metadata,
	)

	// Check if apikey is valid
	if !isValidApikey(splitAPI.SplitFetcher) {
		log.Error.Println("Invalid apikey! Aborting execution.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	redisOptions, err := parseRedisOptions()
	if err != nil {
		logger.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}
	redisClient, err := predis.NewRedisClient(redisOptions, logger)
	if err != nil {
		logger.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(miscStorage, logger)
	if err != nil {
		log.Error.Println("Failed when trying to clean up redis. Aborting execution.")
		log.Error.Println(err.Error())
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	// WebAdmin configuration
	waOptions := &admin.WebAdminOptions{
		Port:          conf.Data.Producer.Admin.Port,
		AdminUsername: conf.Data.Producer.Admin.Username,
		AdminPassword: conf.Data.Producer.Admin.Password,
		DebugOn:       conf.Data.Logger.DebugOn,
	}

	metricStorage := predis.NewMetricsStorage(redisClient, metadata, logger)
	localTelemetryStorage := mutexmap.NewMMMetricsStorage()
	metricsWrapper := storage.NewMetricWrapper(metricStorage, localTelemetryStorage, logger)
	storages := common.Storages{
		SplitStorage:          predis.NewSplitStorage(redisClient, logger),
		SegmentStorage:        predis.NewSegmentStorage(redisClient, logger),
		LocalTelemetryStorage: localTelemetryStorage,
		TelemetryStorage:      metricStorage,
		ImpressionStorage:     predis.NewImpressionStorage(redisClient, dtos.Metadata{}, logger),
		EventStorage:          predis.NewEventsStorage(redisClient, dtos.Metadata{}, logger),
	}
	httpClients := common.HTTPClients{
		SdkClient:    api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.SdkURL, logger, metadata),
		EventsClient: api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.EventsURL, logger, metadata),
	}

	impressionListenerEnabled := strings.TrimSpace(conf.Data.ImpressionListener.Endpoint) != ""
	impressionRecorder := multipleWorkers.NewImpressionRecordMultiple(storages.ImpressionStorage, splitAPI.ImpressionRecorder, metricsWrapper, impressionListenerEnabled, logger)
	eventRecorder := multipleWorkers.NewEventRecorderMultiple(storages.EventStorage, splitAPI.EventRecorder, metricsWrapper, logger)
	workers := synchronizer.Workers{
		SplitFetcher:       worker.NewSplitFetcher(storages.SplitStorage, splitAPI.SplitFetcher, metricsWrapper, logger),
		SegmentFetcher:     worker.NewSegmentFetcher(storages.SplitStorage, storages.SegmentStorage, splitAPI.SegmentFetcher, metricsWrapper, logger),
		EventRecorder:      eventRecorder,
		ImpressionRecorder: impressionRecorder,
		TelemetryRecorder:  multipleWorkers.NewMetricRecorderMultiple(metricsWrapper, splitAPI.MetricRecorder, logger),
	}
	recorders := common.Recorders{
		Impression: impressionRecorder,
		Event:      eventRecorder,
	}

	// Run WebAdmin Server
	admin.StartAdminWebAdmin(waOptions, storages, httpClients, recorders)

	syncImpl := synchronizer.NewSynchronizer(
		config.TaskPeriods{
			CounterSync:    conf.Data.MetricsRefreshRate,
			EventsSync:     conf.Data.EventsPostRate,
			GaugeSync:      conf.Data.MetricsRefreshRate,
			ImpressionSync: conf.Data.ImpressionsPostRate,
			LatencySync:    conf.Data.MetricsRefreshRate,
			SegmentSync:    conf.Data.SegmentFetchRate,
			SplitSync:      conf.Data.SplitsFetchRate,
		},
		advanced,
		workers,
		logger,
		nil,
	)

	managerStatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		syncImpl,
		logger,
		advanced,
		splitAPI.AuthClient,
		storages.SplitStorage,
		managerStatus,
	)
	if err != nil {
		panic(err)
	}

	go syncManager.Start()
	select {
	case status := <-managerStatus:
		switch status {
		case synchronizer.Ready:
			logger.Info("Synchronizer tasks started")
		case synchronizer.Error:
			os.Exit(splitio.ExitTaskInitialization)
		}
	}
	task.InitializeEvictionCalculator()

	// Producer mode - graceful shutdown
	go gracefulShutdownProducer(sigs, gracefulShutdownWaitingGroup, syncManager)

	if impressionListenerEnabled {
		go multipleWorkers.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{Endpoint: conf.Data.ImpressionListener.Endpoint})
	}

	/*
		task.InitializeImpressions(conf.Data.ImpressionsThreads)
		task.InitializeEvents(conf.Data.EventsThreads)
		task.InitializeEvictionCalculator()
		for i := 0; i < conf.Data.ImpressionsThreads; i++ {
			if ilEndpoint := conf.Data.ImpressionListener.Endpoint; ilEndpoint != "" {
				go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{Endpoint: ilEndpoint})
			}
			go task.PostImpressions(
				i,
				impressionsRecorder,
				impressionsStorage,
				conf.Data.ImpressionsPostRate,
				conf.Data.Redis.DisableLegacyImpressions,
				conf.Data.ImpressionListener.Endpoint != "",
				conf.Data.ImpressionsPerPost,
				gracefulShutdownWaitingGroup,
			)

		}

		for i := 0; i < conf.Data.EventsThreads; i++ {
			go task.PostEvents(i, eventsRecorder, eventsStorage, conf.Data.EventsPostRate,
				int(conf.Data.EventsPerPost), gracefulShutdownWaitingGroup)
		}

	*/
	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, storages.SplitStorage, httpClients.SdkClient, httpClients.EventsClient)

	// Keeping service alive
	startLoop(500)
}
