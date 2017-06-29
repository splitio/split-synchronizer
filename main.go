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
	"github.com/splitio/go-agent/splitio/proxy"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/storage"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
	"github.com/splitio/go-agent/splitio/storage/redis"
	"github.com/splitio/go-agent/splitio/task"
)

var asProxy *bool
var benchmarkMode *bool
var configFile *string
var writeDefaultConfigFile *string
var cliParametersMap map[string]interface{}

//------------------------------------------------------------------------------
// MAIN PROGRAM
//------------------------------------------------------------------------------

func init() {
	//Show initial banner
	fmt.Println(splitio.ASCILogo)
	fmt.Println("Split Synchronizer Agent - Version: ", splitio.Version)

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

	if *asProxy {
		var dbpath = boltdb.InMemoryMode
		if conf.Data.Proxy.PersistMemoryPath != "" {
			dbpath = conf.Data.Proxy.PersistMemoryPath
		}
		boltdb.Initialize(dbpath, nil)
	} else {
		redis.Initialize(conf.Data.Redis)
	}

}

func startAsProxy() {
	go task.FetchRawSplits(conf.Data.SplitsFetchRate, conf.Data.SegmentFetchRate)

	//Run webserver loop
	proxy.Run(":" + conf.Data.Proxy.Port)
}

func main() {

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

//------------------------------------------------------------------------------
// Initialization functions
//------------------------------------------------------------------------------

func parseFlags() {
	configFile = flag.String("config", "splitio.agent.conf.json", "a configuration file")
	writeDefaultConfigFile = flag.String("write-default-config", "", "write a default configuration file")
	asProxy = flag.Bool("proxy", false, "run as split server proxy to improve sdk performance")
	benchmarkMode = flag.Bool("benchmark", false, "Benchmark mode")

	// dinamically configuration parameters
	cliParameters := conf.CliParametersToRegister()
	cliParametersMap = make(map[string]interface{}, len(cliParameters))
	for _, param := range cliParameters {
		switch param.AttributeType {
		case "string":
			cliParametersMap[param.Command] = flag.String(param.Command, param.DefaultValue.(string), param.Description)
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
	splitFetcher := splitFetcherFactory()
	splitSorage := splitStorageFactory()
	go task.FetchSplits(splitFetcher, splitSorage, conf.Data.SplitsFetchRate)

	segmentFetcher := segmentFetcherFactory()
	segmentStorage := segmentStorageFactory()
	go task.FetchSegments(segmentFetcher, segmentStorage, conf.Data.SegmentFetchRate)

	for i := 0; i < conf.Data.ImpressionsThreads; i++ {
		impressionsStorage := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
		impressionsRecorder := recorder.ImpressionsHTTPRecorder{}
		go task.PostImpressions(i, impressionsRecorder, impressionsStorage, conf.Data.ImpressionsPostRate)
	}

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
	return fetcher.SegmentFetcherMainFactory{}
}

func segmentStorageFactory() storage.SegmentStorageFactory {
	return storage.SegmentStorageMainFactory{}
}
