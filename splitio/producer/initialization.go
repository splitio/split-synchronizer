package producer

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/util"
	"github.com/splitio/split-synchronizer/splitio/web/admin"
)

func gracefulShutdownProducer(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {
	<-sigs

	log.PostShutdownMessageToSlack(false)

	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Splits - Emit task stop signal
	fmt.Println(" -> Sending STOP to fetch_splits goroutine")
	task.StopFetchSplits()

	// Segments - Emit task stop signal
	fmt.Println(" -> Sending STOP to fetch_segments goroutine")
	task.StopFetchSegments()

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
	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

func startLoop(loopTime int64) {
	for {
		time.Sleep(time.Duration(loopTime) * time.Millisecond)
	}
}

func hashApiKey(apikey string) uint32 {
	return util.Murmur3_32([]byte(apikey), 0)
}

func isApikeyValid(splitFetcher fetcher.HTTPSplitFetcher) bool {
	nowInMillis := time.Now().UnixNano() / int64(time.Millisecond)
	_, err := splitFetcher.Fetch(nowInMillis)
	return err != nil
}

func sanitizeRedis() error {
	miscStorage := redis.NewMiscStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	currentHash := hashApiKey(conf.Data.APIKey)
	currentHashAsStr := strconv.Itoa(int(currentHash))
	defer miscStorage.SetApikeyHash(currentHashAsStr)

	if conf.Data.Redis.ForceFreshStartup {
		log.Warning.Println("Fresh startup requested. Cleaning up redis before initializing.")
		miscStorage.ClearAll()
	}

	previousHashStr, err := miscStorage.GetApikeyHash()
	if err != nil {
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

	// Setup fetchers & recorders
	splitFetcher := fetcher.NewHTTPSplitFetcher()
	segmentFetcher := fetcher.SegmentFetcherMainFactory{}
	impressionsRecorder := recorder.ImpressionsHTTPRecorder{}
	eventsRecorder := recorder.EventsHTTPRecorder{}

	// Setup storages
	splitStorage := redis.NewSplitStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	segmentStorage := redis.SegmentStorageMainFactory{}
	impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	eventsStorage := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)

	if !isApikeyValid(splitFetcher) {
		log.Error.Println("Invalid apikey! Aborting execution.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	// Initialize redis client
	err := redis.Initialize(conf.Data.Redis)
	if err != nil {
		log.Error.Println(err.Error())
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	err = sanitizeRedis()
	if err != nil {
		log.Error.Println("Failed when trying to clean up redis. Aborting execution.")
		os.Exit(splitio.ExitRedisInitializationFailed)
	}

	//Producer mode - graceful shutdown
	go gracefulShutdownProducer(sigs, gracefulShutdownWaitingGroup)

	// WebAdmin configuration
	waOptions := &admin.WebAdminOptions{
		Port:          conf.Data.Producer.Admin.Port,
		AdminUsername: conf.Data.Producer.Admin.Username,
		AdminPassword: conf.Data.Producer.Admin.Password,
		DebugOn:       conf.Data.Logger.DebugOn,
	}
	// Run WebAdmin Server
	admin.StartAdminWebAdmin(waOptions, splitStorage, segmentStorage.NewInstance())

	go task.FetchSplits(splitFetcher, splitStorage, conf.Data.SplitsFetchRate, gracefulShutdownWaitingGroup)
	go task.FetchSegments(segmentFetcher, segmentStorage, conf.Data.SegmentFetchRate, gracefulShutdownWaitingGroup)

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
			conf.Data.EventsPerPost, gracefulShutdownWaitingGroup)
	}

	go task.CheckEnvirontmentStatus(gracefulShutdownWaitingGroup, splitStorage)

	//Keeping service alive
	startLoop(500)
}
