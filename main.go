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
		redis.Initialize(conf.Data.Redis)
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

//------------------------------------------------------------------------------
// MAIN PROGRAM
//------------------------------------------------------------------------------

func main() {

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if *asProxy {
		// Run as proxy using boltdb as in-memoy database
		proxy.Start(sigs, gracefulShutdownWaitingGroup)
	} else {
		// Run as synchronizer using Redis as cache
		producer.Start(sigs, gracefulShutdownWaitingGroup)
	}

}
