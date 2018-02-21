// Split Agent for across Split's SDKs
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/splitio/split-synchronizer/splitio/web/admin/controllers/producer"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/proxy"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web/admin"
)

var asProxy *bool
var benchmarkMode *bool
var versionInfo *bool
var configFile *string
var writeDefaultConfigFile *string
var cliParametersMap map[string]interface{}

var gracefulShutdownWaitingGroup = &sync.WaitGroup{}
var sigs = make(chan os.Signal, 1)

//------------------------------------------------------------------------------
// MAIN PROGRAM
//------------------------------------------------------------------------------

func init() {
	//reading command line options
	parseFlags()

	//print the version
	if *versionInfo {
		fmt.Println("Split Synchronizer - Version: ", splitio.Version)
		os.Exit(0)
	}

	//Show initial banner
	fmt.Println(splitio.ASCILogo)
	fmt.Println("Split Synchronizer - Version: ", splitio.Version)

	//writing a default configuration file if it is required by user
	if *writeDefaultConfigFile != "" {
		fmt.Println("DEFAULT CONFIG FILE HAS BEEN WRITTEN:", *writeDefaultConfigFile)
		conf.WriteDefaultConfigFile(*writeDefaultConfigFile)
		os.Exit(0)
	}

	//Initialize modules
	loadConfiguration()
	loadLogger()
	api.Initialize()

	if *asProxy {
		var dbpath = boltdb.InMemoryMode
		if conf.Data.Proxy.PersistMemoryPath != "" {
			dbpath = conf.Data.Proxy.PersistMemoryPath
		}
		boltdb.Initialize(dbpath, nil)
		stats.Initialize()
	} else {
		redis.Initialize(conf.Data.Redis)
	}

}

func startAsProxy() {
	go task.FetchRawSplits(conf.Data.SplitsFetchRate, conf.Data.SegmentFetchRate)

	if conf.Data.ImpressionListener.Endpoint != "" {
		go task.PostImpressionsToListener(recorder.ImpressionListenerSubmitter{
			Endpoint: conf.Data.ImpressionListener.Endpoint,
		})
	}

	controllers.InitializeImpressionWorkers(
		conf.Data.Proxy.ImpressionsMaxSize,
		int64(conf.Data.ImpressionsPostRate),
	)
	controllers.InitializeEventWorkers(
		conf.Data.Proxy.EventsMaxSize,
		int64(conf.Data.EventsPushRate),
	)

	proxyOptions := &proxy.ProxyOptions{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   conf.Data.Proxy.Auth.APIKeys,
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
	}

	//Run webserver loop
	proxy.Run(proxyOptions)
}

func main() {

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if *asProxy {
		// Run as proxy using boltdb as in-memoy database
		startAsProxy()
	} else {
		// Run as synchronizer using Redis as cache
		startProducer()
		//Keeping service alive
		for {
			time.Sleep(500 * time.Millisecond)
		}
	}

}

func gracefulShutdown() {
	<-sigs
	fmt.Println("\n\n * Starting graceful shutdown")
	fmt.Println("")

	// Splits - Emit task stop signal
	fmt.Println(" -> Sending STOP to fetch_splits gorutine")
	task.StopFetchSplits()

	// Segments - Emit task stop signal
	fmt.Println(" -> Sending STOP to fetch_segments gorutine")
	task.StopFetchSegments()

	// Metrics - Emit task stop signal
	fmt.Println(" -> Sending STOP to post_metrics gorutine")
	task.StopPostMetrics()

	// Events - Emit task stop signal
	for i := 0; i < conf.Data.EventsConsumerThreads; i++ {
		fmt.Println(" -> Sending STOP to post_events gorutine")
		task.StopPostEvents()
	}

	// Impressions - Emit task stop signal
	for i := 0; i < conf.Data.ImpressionsThreads; i++ {
		fmt.Println(" -> Sending STOP to post_impressions gorutine")
		task.StopPostImpressions()
	}

	fmt.Println(" * Waiting gorutines stop")
	gracefulShutdownWaitingGroup.Wait()
	fmt.Println(" * Shutting it down - see you soon!")
	os.Exit(0)
}

//------------------------------------------------------------------------------
// Initialization functions
//------------------------------------------------------------------------------

func parseFlags() {
	configFile = flag.String("config", "splitio.agent.conf.json", "a configuration file")
	writeDefaultConfigFile = flag.String("write-default-config", "", "write a default configuration file")
	asProxy = flag.Bool("proxy", false, "run as split server proxy to improve sdk performance")
	benchmarkMode = flag.Bool("benchmark", false, "Benchmark mode")
	versionInfo = flag.Bool("version", false, "Print the version")

	// dinamically configuration parameters
	cliParameters := conf.CliParametersToRegister()
	cliParametersMap = make(map[string]interface{}, len(cliParameters))
	for _, param := range cliParameters {
		switch param.AttributeType {
		case "string":
			cliParametersMap[param.Command] = flag.String(param.Command, param.DefaultValue.(string), param.Description)
			break
		case "[]string":
			cliParametersMap[param.Command] = flag.String(param.Command, strings.Join(param.DefaultValue.([]string), ","), param.Description)
			break
		case "int":
			cliParametersMap[param.Command] = flag.Int(param.Command, param.DefaultValue.(int), param.Description)
			break
		case "int64":
			cliParametersMap[param.Command] = flag.Int64(param.Command, param.DefaultValue.(int64), param.Description)
			break
		case "bool":
			cliParametersMap[param.Command] = flag.Bool(param.Command, param.DefaultValue.(bool), param.Description)
			break
		}
	}

	flag.Parse()
}

func loadConfiguration() {
	//load default values
	conf.Initialize()
	//overwrite default values from configuration file
	conf.LoadFromFile(*configFile)
	//overwrite with cli values
	conf.LoadFromArgs(cliParametersMap)
}

func loadLogger() {
	var err error

	var commonWriter io.Writer
	var fullWriter io.Writer

	var benchmarkWriter = ioutil.Discard
	var verboseWriter = ioutil.Discard
	var debugWriter = ioutil.Discard
	var fileWriter = ioutil.Discard
	var stdoutWriter = ioutil.Discard
	var slackWriter = ioutil.Discard

	if len(conf.Data.Logger.File) > 3 {
		opt := &log.FileRotateOptions{
			MaxBytes:    conf.Data.Logger.FileMaxSize,
			BackupCount: conf.Data.Logger.FileBackupCount,
			Path:        conf.Data.Logger.File}
		fileWriter, err = log.NewFileRotate(opt)
		if err != nil {
			fmt.Printf("Error opening log file: %s \n", err.Error())
			fileWriter = ioutil.Discard
		} else {
			fmt.Printf("Log file: %s \n", conf.Data.Logger.File)
		}
	}

	if conf.Data.Logger.StdoutOn {
		stdoutWriter = os.Stdout
	}

	_, err = url.ParseRequestURI(conf.Data.Logger.SlackWebhookURL)
	if err == nil {
		slackWriter = &log.SlackWriter{WebHookURL: conf.Data.Logger.SlackWebhookURL, Channel: conf.Data.Logger.SlackChannel, RefreshRate: 30}
	}

	commonWriter = io.MultiWriter(stdoutWriter, fileWriter)
	fullWriter = io.MultiWriter(commonWriter, slackWriter)

	if conf.Data.Logger.VerboseOn {
		verboseWriter = commonWriter
	}

	if conf.Data.Logger.DebugOn {
		debugWriter = commonWriter
	}

	if *benchmarkMode == true {
		benchmarkWriter = commonWriter
	}

	log.Initialize(benchmarkWriter, verboseWriter, debugWriter, commonWriter, commonWriter, fullWriter)
}

func startProducer() {

	task.InitializeEvents(conf.Data.EventsConsumerThreads)
	task.InitializeImpressions(conf.Data.ImpressionsThreads)

	//Producer mode - graceful shutdown
	go gracefulShutdown()

	splitFetcher := splitFetcherFactory()
	splitStorage := splitStorageFactory()

	go func() {
		// WebAdmin configuration
		waOptions := &admin.WebAdminOptions{
			Port:          conf.Data.Producer.Admin.Port,
			AdminUsername: conf.Data.Producer.Admin.Username,
			AdminPassword: conf.Data.Producer.Admin.Password,
			DebugOn:       conf.Data.Logger.DebugOn,
		}

		waServer := admin.NewWebAdminServer(waOptions)

		waServer.Router().Use(func(c *gin.Context) {
			c.Set("SplitStorage", splitStorage)
		})

		waServer.Router().GET("/admin/healthcheck", producer.HealthCheck)

		waServer.Run()
	}()

	go task.FetchSplits(splitFetcher, splitStorage, conf.Data.SplitsFetchRate, gracefulShutdownWaitingGroup)

	segmentFetcher := segmentFetcherFactory()
	segmentStorage := segmentStorageFactory()
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
			conf.Data.ImpressionListener.Endpoint != "",
			gracefulShutdownWaitingGroup,
		)

	}

	metricsStorage := redis.NewMetricsStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	metricsRecorder := recorder.MetricsHTTPRecorder{}
	go task.PostMetrics(metricsRecorder, metricsStorage, conf.Data.MetricsPostRate, gracefulShutdownWaitingGroup)

	for i := 0; i < conf.Data.EventsConsumerThreads; i++ {
		eventsStorage := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
		eventsRecorder := recorder.EventsHTTPRecorder{}
		go task.PostEvents(i, eventsRecorder, eventsStorage, conf.Data.EventsPushRate,
			conf.Data.EventsConsumerReadSize, gracefulShutdownWaitingGroup)
	}
}

func flushEvents() {
	fmt.Println("Starting to flush events")
	eventsStorage := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	eventsRecorder := recorder.EventsHTTPRecorder{}
	go task.EventsFlush(eventsRecorder, eventsStorage, conf.Data.EventsConsumerReadSize)
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
