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
	"strings"
	"sync"
	"syscall"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/splitio/producer"
	"github.com/splitio/split-synchronizer/splitio/proxy"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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
// Go Initialization
//------------------------------------------------------------------------------

func checkDeprecatedConfigParameters() []string {
	deprecatedMessages := make([]string, 0)

	if conf.Data.ImpressionsConsumerThreads > 0 {
		deprecatedMessages = append(deprecatedMessages, "The cli parameter 'impressions-consumer-threads' will be deprecated soon in favor of 'impressions-threads'. Mapping to replacement: 'impressions-threads'.")
		if conf.Data.ImpressionsThreads == 1 {
			conf.Data.ImpressionsThreads = conf.Data.ImpressionsConsumerThreads
		}
	}

	if conf.Data.EventsConsumerReadSize > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsConsumerReadSize' and 'events-consumer-read-size' will be deprecated soon in favor of 'eventsPerPost' or 'events-per-post'. Mapping to replacement: 'eventsPerPost'/'events-per-post'.")
		if conf.Data.EventsPerPost == 10000 {
			conf.Data.EventsPerPost = conf.Data.EventsConsumerReadSize
		}
	}

	if conf.Data.EventsPushRate > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsPushRate' and 'events-push-rate' will be deprecated soon in favor of 'eventsPostRate' or 'events-post-rate'. Mapping to replacement: 'eventsPostRate'/'events-post-rate'.")
		if conf.Data.EventsPostRate == 60 {
			conf.Data.EventsPostRate = conf.Data.EventsPushRate
		}
	}

	if conf.Data.ImpressionsRefreshRate > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'impressionsRefreshRate' will be deprecated soon in favor of 'impressionsPostRate'. Mapping to replacement: 'impressionsPostRate'.")
		if conf.Data.ImpressionsPostRate == 20 {
			conf.Data.ImpressionsPostRate = conf.Data.ImpressionsRefreshRate
		}
	}

	if conf.Data.EventsConsumerThreads > 0 {
		deprecatedMessages = append(deprecatedMessages, "The parameter 'eventsConsumerThreads' and 'events-consumer-threads' will be deprecated soon in favor of 'eventsThreads' or 'events-threads'. Mapping to replacement 'eventsThreads'/'events-threads'.")
		if conf.Data.EventsThreads == 1 {
			conf.Data.EventsThreads = conf.Data.EventsConsumerThreads
		}
	}

	return deprecatedMessages
}

func main() {
	//reading command line options
	parseFlags()

	//print the version
	if *versionInfo {
		fmt.Printf("\nSplit Synchronizer - Version: %s (%s) \n", splitio.Version, splitio.CommitVersion)
		os.Exit(splitio.SuccessfulOperation)
	}

	//Show initial banner
	fmt.Println(splitio.ASCILogo)
	fmt.Printf("\nSplit Synchronizer - Version: %s (%s) \n", splitio.Version, splitio.CommitVersion)

	//writing a default configuration file if it is required by user
	if *writeDefaultConfigFile != "" {
		conf.WriteDefaultConfigFile(*writeDefaultConfigFile)
		os.Exit(splitio.SuccessfulOperation)
	}

	//Initialize modules
	err := loadConfiguration()
	if err != nil {
		os.Exit(splitio.ExitInvalidConfiguration)
	}
	loadLogger()

	deprecatedMessages := checkDeprecatedConfigParameters()
	if len(deprecatedMessages) > 0 {
		for _, msg := range deprecatedMessages {
			log.Warning.Println(msg)
		}
	}

	api.Initialize()
	stats.Initialize()

	if *asProxy {
		appcontext.Initialize(appcontext.ProxyMode)

		var dbpath = boltdb.InMemoryMode
		if conf.Data.Proxy.PersistMemoryPath != "" {
			dbpath = conf.Data.Proxy.PersistMemoryPath
		}
		boltdb.Initialize(dbpath, nil)
	} else {
		appcontext.Initialize(appcontext.ProducerMode)
		err := redis.Initialize(conf.Data.Redis)
		if err != nil {
			log.Error.Println(err.Error())
			os.Exit(splitio.ExitRedisInitializationFailed)
		}
	}

	log.PostStartedMessageToSlack()

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if *asProxy {
		// Run as proxy using boltdb as in-memoy database
		proxy.Start(sigs, gracefulShutdownWaitingGroup)
	} else {
		// Run as synchronizer using Redis as cache
		producer.Start(sigs, gracefulShutdownWaitingGroup)
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

func loadConfiguration() error {
	//load default values
	conf.Initialize()
	//overwrite default values from configuration file
	err := conf.LoadFromFile(*configFile)
	if err != nil {
		return err
	}
	//overwrite with cli values
	conf.LoadFromArgs(cliParametersMap)

	return nil
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
