package producer

import (
	"errors"
	"fmt"
	l "log"
	"os"
	"strconv"
	"sync"
	"time"

	config "github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	predis "github.com/splitio/go-split-commons/storage/redis"
	"github.com/splitio/go-split-commons/synchronizer"
	"github.com/splitio/go-split-commons/synchronizer/worker"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	multipleWorkers "github.com/splitio/split-synchronizer/splitio/producer/worker"
	"github.com/splitio/split-synchronizer/splitio/util"
	"github.com/splitio/split-synchronizer/splitio/web/admin"
)

func gracefulShutdownProducer(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup, syncManager *synchronizer.Manager) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	syncManager.Stop()

	/*
		// Metrics - Emit task stop signal
		fmt.Println(" -> Sending STOP to post_metrics goroutine")
		task.StopPostMetrics()

		// Events - Emit task stop signal
		for i := 0; i < conf.Data.EventsThreads; i++ {
			fmt.Println(" -> Sending STOP to post_events goroutine")
			task.StopPostEvents()
		}

		// Impressions - Emit task stop signal
		for i := 0; i < conf.Data.ImpressionsThreads; i++ {
			fmt.Println(" -> Sending STOP to post_impressions goroutine")
			task.StopPostImpressions()
		}

		// Healthcheck - Emit task stop signal
		fmt.Println(" -> Sending STOP to healthcheck goroutine")
		task.StopHealtcheck()

		fmt.Println(" * Waiting goroutines stop")
		gracefulShutdownWaitingGroup.Wait()
	*/
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

func sanitizeRedis(miscStorage *predis.MiscStorage) error {
	if miscStorage == nil {
		return errors.New("Could not sanitize redis")
	}
	currentHash := hashAPIKey(conf.Data.APIKey)
	currentHashAsStr := strconv.Itoa(int(currentHash))
	defer miscStorage.SetApikeyHash(currentHashAsStr)

	if conf.Data.Redis.ForceFreshStartup {
		log.Warning.Println("Fresh startup requested. Cleaning up redis before initializing.")
		miscStorage.ClearAll()
		return nil
	}

	previousHashStr, err := miscStorage.GetApikeyHash()
	if err != nil && err.Error() != predis.ErrorHashNotPresent { // Missing hash is not considered an error
		return err
	}

	if currentHashAsStr != previousHashStr {
		log.Warning.Println("Previous apikey is missing/different from current one. Cleaning up redis before startup.")
		miscStorage.ClearAll()
	}
	return nil
}

// Start initialize the producer mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	logger := logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: l.LUTC | l.Ldate | l.Lmicroseconds | l.Lshortfile,
		LogLevel:            logging.LevelInfo,
	})
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

	/*
		err := api.ValidateApikey(conf.Data.APIKey, *advanced)
		if err != nil {
			log.Error.Println("Invalid apikey! Aborting execution.")
			os.Exit(splitio.ExitRedisInitializationFailed)
		}
	*/

	redisClient, err := predis.NewRedisClient(&config.RedisConfig{
		Host:   "localhost",
		Port:   6379,
		Prefix: conf.Data.Redis.Prefix,
	}, logger)
	if err != nil {
		logger.Error("Failed to instantiate redis client.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	// impressionsRecorder := recorder.ImpressionsHTTPRecorder{}
	// eventsRecorder := recorder.EventsHTTPRecorder{}

	// Setup storages
	// impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	// eventsStorage := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(miscStorage)
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

	// What should I do here?
	// Sync is storing
	// test-poc:.SPLITIO///count.segmentChangeFetcher.status.200
	// test-poc:.SPLITIO///count.splitChangeFetcher.status.200
	// test-poc:.SPLITIO///latency.segmentChangeFetcher.time.bucket.22
	metadata := dtos.Metadata{
		MachineIP:   "some",
		MachineName: "some",
		SDKVersion:  "split-synchronizer",
	}

	splitStorage := predis.NewSplitStorage(redisClient, logger)
	segmentStorage := predis.NewSegmentStorage(redisClient, logger)
	metricStorage := predis.NewMetricsStorage(redisClient, metadata, logger)
	impressionStorage := predis.NewImpressionStorage(redisClient, dtos.Metadata{}, logger)
	eventStorage := predis.NewEventsStorage(redisClient, dtos.Metadata{}, logger)
	localTelemetryStorage := mutexmap.NewMMMetricsStorage()

	// Run WebAdmin Server
	// admin.StartAdminWebAdmin(waOptions, splitStorage, segmentStorage.NewInstance())
	admin.StartAdminWebAdmin(waOptions, splitStorage, segmentStorage, eventStorage, impressionStorage)

	// Setup fetchers & recorders
	splitAPI := service.NewSplitAPI(
		conf.Data.APIKey,
		advanced,
		logger,
	)

	workers := synchronizer.Workers{
		SplitFetcher:       worker.NewSplitFetcher(splitStorage, splitAPI.SplitFetcher, localTelemetryStorage, logger),
		SegmentFetcher:     worker.NewSegmentFetcher(splitStorage, segmentStorage, splitAPI.SegmentFetcher, localTelemetryStorage, logger),
		EventRecorder:      multipleWorkers.NewEventRecorderMultiple(eventStorage, splitAPI.EventRecorder, localTelemetryStorage, logger),
		ImpressionRecorder: multipleWorkers.NewImpressionRecordMultiple(impressionStorage, splitAPI.ImpressionRecorder, localTelemetryStorage, logger),
		TelemetryRecorder:  worker.NewMetricRecorder(metricStorage, splitAPI.MetricRecorder, dtos.Metadata{}),
	}

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
		splitStorage,
		managerStatus,
	)
	if err != nil {
		panic(err)
	}

	go syncManager.Start()

	//Producer mode - graceful shutdown
	go gracefulShutdownProducer(sigs, gracefulShutdownWaitingGroup, syncManager)

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

		metricsStorage := redis.NewMetricsStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
		metricsRecorder := recorder.MetricsHTTPRecorder{}
		go task.PostMetrics(metricsRecorder, metricsStorage, conf.Data.MetricsPostRate, gracefulShutdownWaitingGroup)

		for i := 0; i < conf.Data.EventsThreads; i++ {
			go task.PostEvents(i, eventsRecorder, eventsStorage, conf.Data.EventsPostRate,
				int(conf.Data.EventsPerPost), gracefulShutdownWaitingGroup)
		}

	*/
	// go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, splitStorage)

	//Keeping service alive
	startLoop(500)
}
