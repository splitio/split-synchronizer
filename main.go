// Split Agent for across Split's SDKs
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/fetcher"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
	"github.com/splitio/go-agent/splitio/storage/redis"
	"github.com/splitio/go-agent/splitio/task"
)

var configFile *string
var writeDefaultConfigFile *string

//------------------------------------------------------------------------------
// MAIN PROGRAM
//------------------------------------------------------------------------------

func init() {
	//Show initial banner
	fmt.Println(splitio.ASCILogo)
	fmt.Println("Split Software Agent - Version: ", splitio.Version)

	//reading command line options
	parseFlags()

	//writing a default configuration file if it is required by user
	if *writeDefaultConfigFile != "" {
		fmt.Println("DEFAULT CONFIG FILE HAS BEEN WROTE:", *writeDefaultConfigFile)
		conf.WriteDefaultConfigFile(*writeDefaultConfigFile)
		os.Exit(0)
	}

	//Initialize modules
	loadConfiguration()
	loadLogger()
	api.Initialize()
	redis.Initialize(conf.Data.Redis.Host, conf.Data.Redis.Port,
		conf.Data.Redis.Pass, conf.Data.Redis.Db)
}

func main() {
	startProducer()

	//Keeping service alive
	for {
		time.Sleep(500 * time.Millisecond)
	}
}

//------------------------------------------------------------------------------
// Initialization functions
//------------------------------------------------------------------------------

func parseFlags() {
	configFile = flag.String("config", "splitio.agent.conf.json", "a configuration file")
	writeDefaultConfigFile = flag.String("write-default-config", "", "write a default configuration file")
	flag.Parse()
}

func loadConfiguration() {
	conf.Load(*configFile)
}

func loadLogger() {
	var err error

	var commonWriter io.Writer
	var fullWriter io.Writer

	var verboseWriter = ioutil.Discard
	var debugWriter = ioutil.Discard
	var fileWriter = ioutil.Discard
	var stdoutWriter = ioutil.Discard
	var slackWriter = ioutil.Discard

	if len(conf.Data.Logger.File) > 3 {
		fileWriter, err = os.OpenFile(conf.Data.Logger.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Error opening log file: %s \n", err.Error())
		} else {
			fmt.Printf("Log file: %s \n", conf.Data.Logger.File)
		}
	}

	if conf.Data.Logger.StdoutOn {
		stdoutWriter = os.Stdout
	}

	_, err = url.ParseRequestURI(conf.Data.Logger.SlackWebhookURL)
	if err == nil {
		slackWriter = &log.SlackWriter{WebHookURL: conf.Data.Logger.SlackWebhookURL, Channel: conf.Data.Logger.SlackChannel}
	}

	commonWriter = io.MultiWriter(stdoutWriter, fileWriter)
	fullWriter = io.MultiWriter(commonWriter, slackWriter)

	if conf.Data.Logger.VerboseOn {
		verboseWriter = commonWriter
	}

	if conf.Data.Logger.DebugOn {
		debugWriter = commonWriter
	}

	log.Initialize(verboseWriter, debugWriter, commonWriter, commonWriter, fullWriter)
}

func startProducer() {

	splitFetcher := splitFetcherFactory()
	splitSorage := splitStorageFactory()
	go task.FetchSplits(splitFetcher, splitSorage, conf.Data.SplitsFetchRate)

	segmentFetcher := segmentFetcherFactory()
	segmentStorage := segmentStorageFactory()
	go task.FetchSegments(segmentFetcher, segmentStorage, conf.Data.SegmentFetchRate)

	impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	impressionsRecorder := recorder.ImpressionsHTTPRecorder{}
	go task.PostImpressions(impressionsRecorder, impressionsStorage, conf.Data.ImpressionsPostRate)

	metricsStorage := redis.NewMetricsStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	metricsRecorder := recorder.MetricsHTTPRecorder{}
	go task.PostMetrics(metricsRecorder, metricsStorage, conf.Data.MetricsPostRate)
}

func splitFetcherFactory() fetcher.SplitFetcher {
	return fetcher.NewHTTPSplitFetcher()
}

func splitStorageFactory() storage.SplitStorage {
	return redis.NewSplitStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}

func segmentFetcherFactory() fetcher.SegmentFetcherFactory {
	return fetcher.SegmentFetcherFactory{}
}

func segmentStorageFactory() storage.SegmentStorageFactory {
	//return redis.NewSegmentStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	return storage.SegmentStorageFactory{}
}
