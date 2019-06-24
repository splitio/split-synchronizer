package producer

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/task"
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

	fmt.Println(" * Waiting goroutines stop")
	gracefulShutdownWaitingGroup.Wait()
	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(splitio.SuccessfulOperation)
}

func splitFetcherFactory() fetcher.SplitFetcher {
	return fetcher.NewHTTPSplitFetcher()
}

func splitStorageFactory() storage.SplitStorage {
	return redis.NewSplitStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}

func segmentFetcherFactory() fetcher.SegmentFetcherFactory {
	return fetcher.SegmentFetcherMainFactory{}
}

func segmentStorageFactory() storage.SegmentStorageFactory {
	return storage.SegmentStorageMainFactory{}
}

func trafficTypeStorageFactory() storage.TrafficTypeStorage {
	return redis.NewTrafficTypeStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}

func startLoop(loopTime int64) {
	for {
		time.Sleep(time.Duration(loopTime) * time.Millisecond)
	}
}

// Start initialize the producer mode
func Start(sigs chan os.Signal, gracefulShutdownWaitingGroup *sync.WaitGroup) {

	task.InitializeEvents(conf.Data.EventsThreads)
	task.InitializeImpressions(conf.Data.ImpressionsThreads)

	//Producer mode - graceful shutdown
	go gracefulShutdownProducer(sigs, gracefulShutdownWaitingGroup)

	splitFetcher := splitFetcherFactory()
	splitStorage := splitStorageFactory()

	segmentFetcher := segmentFetcherFactory()
	segmentStorage := segmentStorageFactory()

	trafficTypeStorage := trafficTypeStorageFactory()

	// WebAdmin configuration
	waOptions := &admin.WebAdminOptions{
		Port:          conf.Data.Producer.Admin.Port,
		AdminUsername: conf.Data.Producer.Admin.Username,
		AdminPassword: conf.Data.Producer.Admin.Password,
		DebugOn:       conf.Data.Logger.DebugOn,
	}
	// Run WebAdmin Server
	admin.StartAdminWebAdmin(waOptions, splitStorage, segmentStorage.NewInstance())

	go task.FetchSplits(splitFetcher, splitStorage, conf.Data.SplitsFetchRate, gracefulShutdownWaitingGroup, trafficTypeStorage)

	go task.FetchSegments(segmentFetcher, segmentStorage, conf.Data.SegmentFetchRate, gracefulShutdownWaitingGroup)

	for i := 0; i < conf.Data.ImpressionsThreads; i++ {
		impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
		impressionsRecorder := recorder.ImpressionsHTTPRecorder{}
		if conf.Data.ImpressionListener.Endpoint != "" {
			go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
				Endpoint: conf.Data.ImpressionListener.Endpoint,
			})
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
		eventsStorage := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
		eventsRecorder := recorder.EventsHTTPRecorder{}
		go task.PostEvents(i, eventsRecorder, eventsStorage, conf.Data.EventsPostRate,
			conf.Data.EventsPerPost, gracefulShutdownWaitingGroup)
	}

	//Keeping service alive
	startLoop(500)
}
