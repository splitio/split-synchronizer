package producer

import (
	"fmt"
	"os"
	"strings"
	"sync"

	cfg "github.com/splitio/go-split-commons/v3/conf"
	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-split-commons/v3/provisional"
	"github.com/splitio/go-split-commons/v3/service"
	"github.com/splitio/go-split-commons/v3/service/api"
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-split-commons/v3/storage/mutexmap"
	"github.com/splitio/go-split-commons/v3/storage/redis"
	"github.com/splitio/go-split-commons/v3/synchronizer"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/impressionscount"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/segment"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/split"
	"github.com/splitio/go-split-commons/v3/tasks"
	"github.com/splitio/go-toolkit/v4/logging"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	sprov "github.com/splitio/split-synchronizer/v4/splitio/producer/provisional"
	"github.com/splitio/split-synchronizer/v4/splitio/producer/worker"
	"github.com/splitio/split-synchronizer/v4/splitio/recorder"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
	"github.com/splitio/split-synchronizer/v4/splitio/web/admin"
)

func gracefulShutdownProducer(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager synchronizer.Manager) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Stopping Sync Manager in charge of PeriodicFetchers and PeriodicRecorders as well as Streaming
	fmt.Println(" -> Sending STOP to Synchronizer")
	syncManager.Stop()

	// Healthcheck - Emit task stop signal
	fmt.Println(" -> Sending STOP to healthcheck goroutine")
	task.StopHealtcheck()

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()

	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

// Start initialize the producer mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	// Getting initial config data
	advanced := conf.ParseAdvancedOptions()
	metadata := util.GetMetadata()

	// Setup fetchers & recorders
	splitAPI := service.NewSplitAPI(
		conf.Data.APIKey,
		advanced,
		log.Instance,
		metadata,
	)

	// Check if apikey is valid
	if !isValidApikey(splitAPI.SplitFetcher) {
		log.Instance.Error("Invalid apikey! Aborting execution.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	// Redis Storages
	redisOptions, err := parseRedisOptions()
	if err != nil {
		log.Instance.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}
	redisClient, err := redis.NewRedisClient(redisOptions, log.Instance)
	if err != nil {
		log.Instance.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	// Instantiating storages
	miscStorage := redis.NewMiscStorage(redisClient, log.Instance)
	err = sanitizeRedis(miscStorage, log.Instance)
	if err != nil {
		log.Instance.Error("Failed when trying to clean up redis. Aborting execution.")
		log.Instance.Error(err.Error())
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	metricStorage := redis.NewMetricsStorage(redisClient, metadata, log.Instance)
	localTelemetryStorage := mutexmap.NewMMMetricsStorage()
	metricsWrapper := storage.NewMetricWrapper(metricStorage, localTelemetryStorage, log.Instance)
	storages := common.Storages{
		SplitStorage:          redis.NewSplitStorage(redisClient, log.Instance),
		SegmentStorage:        redis.NewSegmentStorage(redisClient, log.Instance),
		LocalTelemetryStorage: localTelemetryStorage,
		ImpressionStorage:     redis.NewImpressionStorage(redisClient, dtos.Metadata{}, log.Instance),
		EventStorage:          redis.NewEventsStorage(redisClient, dtos.Metadata{}, log.Instance),
	}

	// Creating Workers and Tasks
	eventRecorder := worker.NewEventRecorderMultiple(storages.EventStorage, splitAPI.EventRecorder, metricsWrapper, log.Instance)
	workers := synchronizer.Workers{
		SplitFetcher:      split.NewSplitFetcher(storages.SplitStorage, splitAPI.SplitFetcher, metricsWrapper, log.Instance),
		SegmentFetcher:    segment.NewSegmentFetcher(storages.SplitStorage, storages.SegmentStorage, splitAPI.SegmentFetcher, metricsWrapper, log.Instance),
		EventRecorder:     eventRecorder,
		TelemetryRecorder: worker.NewMetricRecorderMultiple(metricsWrapper, splitAPI.MetricRecorder, log.Instance),
	}
	splitTasks := synchronizer.SplitTasks{
		SplitSyncTask:     tasks.NewFetchSplitsTask(workers.SplitFetcher, conf.Data.SplitsFetchRate, log.Instance),
		SegmentSyncTask:   tasks.NewFetchSegmentsTask(workers.SegmentFetcher, conf.Data.SegmentFetchRate, advanced.SegmentWorkers, advanced.SegmentQueueSize, log.Instance),
		TelemetrySyncTask: tasks.NewRecordTelemetryTask(workers.TelemetryRecorder, conf.Data.MetricsPostRate, log.Instance),
		EventSyncTask:     tasks.NewRecordEventsTasks(workers.EventRecorder, advanced.EventsBulkSize, conf.Data.EventsPostRate, log.Instance, conf.Data.EventsThreads),
	}

	impressionListenerEnabled := strings.TrimSpace(conf.Data.ImpressionListener.Endpoint) != ""
	managerConfig := cfg.ManagerConfig{
		ImpressionsMode: conf.Data.ImpressionsMode,
		OperationMode:   cfg.ProducerSync,
		ListenerEnabled: impressionListenerEnabled,
	}

	var impressionsCounter *provisional.ImpressionsCounter
	if conf.Data.ImpressionsMode == cfg.ImpressionsModeOptimized {
		impressionsCounter = provisional.NewImpressionsCounter()
		workers.ImpressionsCountRecorder = impressionscount.NewRecorderSingle(impressionsCounter, splitAPI.ImpressionRecorder, metadata, log.Instance)
		splitTasks.ImpressionsCountSyncTask = tasks.NewRecordImpressionsCountTask(workers.ImpressionsCountRecorder, log.Instance)
	}
	impressionRecorder, err := worker.NewImpressionRecordMultiple(storages.ImpressionStorage, splitAPI.ImpressionRecorder, metricsWrapper, log.Instance, managerConfig, impressionsCounter)
	if err != nil {
		log.Instance.Error(err)
		os.Exit(splitio.ExitTaskInitialization)
	}
	//splitTasks.ImpressionSyncTask = tasks.NewRecordImpressionsTasks(impressionRecorder, conf.Data.ImpressionsPostRate, log.Instance, advanced.ImpressionsBulkSize, conf.Data.ImpressionsThreads)
	splitTasks.ImpressionSyncTask = sprov.NewImpressionsEvictioner(
		storages.ImpressionStorage,
		logging.NewLogger(nil),
		sprov.Config{
			Apikey:     conf.Data.APIKey,
			EventsHost: advanced.EventsURL,
		},
	)

	// Creating Synchronizer for tasks
	syncImpl := synchronizer.NewSynchronizer(
		advanced,
		splitTasks,
		workers,
		log.Instance,
		nil,
	)
	managerStatus := make(chan int, 1)
	syncManager, err := synchronizer.NewSynchronizerManager(
		syncImpl,
		log.Instance,
		advanced,
		splitAPI.AuthClient,
		storages.SplitStorage,
		managerStatus,
	)
	if err != nil {
		log.Instance.Error(err)
		os.Exit(splitio.ExitTaskInitialization)
	}

	// Producer mode - graceful shutdown
	go gracefulShutdownProducer(sigs, gracefulShutdownWaitingGroup, syncManager)

	// --------------------------- ADMIN DASHBOARD ------------------------------
	// WebAdmin configuration
	waOptions := &admin.WebAdminOptions{
		Port:          conf.Data.Producer.Admin.Port,
		AdminUsername: conf.Data.Producer.Admin.Username,
		AdminPassword: conf.Data.Producer.Admin.Password,
		DebugOn:       conf.Data.Logger.DebugOn,
	}

	// Run WebAdmin Server
	httpClients := common.HTTPClients{
		AuthClient:   api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.AuthServiceURL, log.Instance, metadata),
		SdkClient:    api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.SdkURL, log.Instance, metadata),
		EventsClient: api.NewHTTPClient(conf.Data.APIKey, advanced, advanced.EventsURL, log.Instance, metadata),
	}
	recorders := common.Recorders{
		Impression: impressionRecorder,
		Event:      eventRecorder,
	}
	admin.StartAdminWebAdmin(waOptions, storages, httpClients, recorders)
	// ---------------------------------------------------------------------------

	// Run Sync Manager
	go syncManager.Start()
	select {
	case status := <-managerStatus:
		switch status {
		case synchronizer.Ready:
			log.Instance.Info("Synchronizer tasks started")
		case synchronizer.Error:
			log.Instance.Error("Error starting synchronizer")
			os.Exit(splitio.ExitTaskInitialization)
		}
	}
	task.InitializeEvictionCalculator()

	if impressionListenerEnabled {
		for i := 0; i < conf.Data.ImpressionsThreads; i++ {
			go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{Endpoint: conf.Data.ImpressionListener.Endpoint})
		}
	}

	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, storages.SplitStorage, httpClients)

	// Keeping service alive
	startLoop(500)
}
