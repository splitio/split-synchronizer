package producer

import (
	"fmt"
	l "log"
	"os"
	"strings"
	"sync"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	"github.com/splitio/go-split-commons/storage/redis"
	"github.com/splitio/go-split-commons/synchronizer"
	"github.com/splitio/go-split-commons/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/synchronizer/worker/split"
	"github.com/splitio/go-split-commons/tasks"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/common"
	multipleWorkers "github.com/splitio/split-synchronizer/splitio/producer/worker"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/task"
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

// Start initialize the producer mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	logger := logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: l.LUTC | l.Ldate | l.Lmicroseconds | l.Lshortfile,
		LogLevel:            logging.LevelInfo,
	})

	advanced := getConfig()
	metadata := getMetadata()

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
	redisClient, err := redis.NewRedisClient(redisOptions, logger)
	if err != nil {
		logger.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	miscStorage := redis.NewMiscStorage(redisClient, logger)
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

	metricStorage := redis.NewMetricsStorage(redisClient, metadata, logger)
	localTelemetryStorage := mutexmap.NewMMMetricsStorage()
	metricsWrapper := storage.NewMetricWrapper(metricStorage, localTelemetryStorage, logger)
	storages := common.Storages{
		SplitStorage:          redis.NewSplitStorage(redisClient, logger),
		SegmentStorage:        redis.NewSegmentStorage(redisClient, logger),
		LocalTelemetryStorage: localTelemetryStorage,
		ImpressionStorage:     redis.NewImpressionStorage(redisClient, dtos.Metadata{}, logger),
		EventStorage:          redis.NewEventsStorage(redisClient, dtos.Metadata{}, logger),
	}
	httpClients := common.HTTPClients{
		SdkClient:    api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.SdkURL, logger, metadata),
		EventsClient: api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.EventsURL, logger, metadata),
	}

	impressionListenerEnabled := strings.TrimSpace(conf.Data.ImpressionListener.Endpoint) != ""
	impressionRecorder := multipleWorkers.NewImpressionRecordMultiple(storages.ImpressionStorage, splitAPI.ImpressionRecorder, metricsWrapper, impressionListenerEnabled, logger)
	eventRecorder := multipleWorkers.NewEventRecorderMultiple(storages.EventStorage, splitAPI.EventRecorder, metricsWrapper, logger)
	workers := synchronizer.Workers{
		SplitFetcher:       split.NewSplitFetcher(storages.SplitStorage, splitAPI.SplitFetcher, metricsWrapper, logger),
		SegmentFetcher:     segment.NewSegmentFetcher(storages.SplitStorage, storages.SegmentStorage, splitAPI.SegmentFetcher, metricsWrapper, logger),
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

	splitTasks := synchronizer.SplitTasks{
		SplitSyncTask:      tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, logger),
		SegmentSyncTask:    tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers, advanced.SegmentQueueSize, logger),
		TelemetrySyncTask:  tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsRefreshRate, logger),
		EventSyncTask:      tasks.NewRecordEventsTasks(workers.EventRecorder, advanced.EventsBulkSize, conf.Data.EventsPostRate, logger, conf.Data.EventsThreads),
		ImpressionSyncTask: tasks.NewRecordImpressionsTasks(workers.ImpressionRecorder, conf.Data.ImpressionsPostRate, logger, advanced.ImpressionsBulkSize, conf.Data.ImpressionsThreads),
	}

	syncImpl := synchronizer.NewSynchronizer(
		advanced,
		splitTasks,
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
		for i := 0; i < conf.Data.ImpressionsThreads; i++ {
			go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{Endpoint: conf.Data.ImpressionListener.Endpoint})
		}
	}

	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, storages.SplitStorage, httpClients.SdkClient, httpClients.EventsClient)

	// Keeping service alive
	startLoop(500)
}
