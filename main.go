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
)

type configMap map[string]interface{}
type flagInformation struct {
	configFile             *string
	writeDefaultConfigFile *string
	asProxy                *bool
	benchmarkMode          *bool
	versionInfo            *bool
	cliParametersMap       configMap
}

var gracefulShutdownWaitingGroup = &sync.WaitGroup{}
var sigs = make(chan os.Signal, 1)

func checkDeprecatedConfigParameters() {
	deprecatedMessages := conf.ProcessDeprecatedOptions()
	if len(deprecatedMessages) > 0 {
		for _, msg := range deprecatedMessages {
			log.Warning.Println(msg)
		}
	}
}

func parseCLIFlags() *flagInformation {
	cliFlags := &flagInformation{
		configFile:             flag.String("config", "splitio.agent.conf.json", "a configuration file"),
		writeDefaultConfigFile: flag.String("write-default-config", "", "write a default configuration file"),
		asProxy:                flag.Bool("proxy", false, "run as split server proxy to improve sdk performance"),
		benchmarkMode:          flag.Bool("benchmark", false, "Benchmark mode"),
		versionInfo:            flag.Bool("version", false, "Print the version"),
	}

	// dinamically configuration parameters
	cliParameters := conf.CliParametersToRegister()
	cliParametersMap := make(configMap, len(cliParameters))
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

	cliFlags.cliParametersMap = cliParametersMap
	flag.Parse()
	return cliFlags
}

func loadConfiguration(configFile *string, cliParametersMap configMap) error {
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

func loadLogger(benchmarkMode *bool) {
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

func main() {
	//reading command line options
	cliFlags := parseCLIFlags()

	//print the version
	if *cliFlags.versionInfo {
		fmt.Printf("\nSplit Synchronizer - Version: %s (%s) \n", splitio.Version, splitio.CommitVersion)
		os.Exit(splitio.SuccessfulOperation)
	}

	//Show initial banner
	fmt.Println(splitio.ASCILogo)
	fmt.Printf("\nSplit Synchronizer - Version: %s (%s) \n", splitio.Version, splitio.CommitVersion)

	//writing a default configuration file if it is required by user
	if *cliFlags.writeDefaultConfigFile != "" {
		conf.WriteDefaultConfigFile(*cliFlags.writeDefaultConfigFile)
		os.Exit(splitio.SuccessfulOperation)
	}

	//Initialize modules
	err := loadConfiguration(cliFlags.configFile, cliFlags.cliParametersMap)
	if err != nil {
		os.Exit(splitio.ExitInvalidConfiguration)
	}

	// These functions rely on the config module being successfully populated
	loadLogger(cliFlags.benchmarkMode)
	checkDeprecatedConfigParameters()
	api.Initialize()
	stats.Initialize()

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if *cliFlags.asProxy {
		appcontext.Initialize(appcontext.ProxyMode)
		log.PostStartedMessageToSlack()
		proxy.Start(sigs, gracefulShutdownWaitingGroup)
	} else {
		appcontext.Initialize(appcontext.ProducerMode)
		log.PostStartedMessageToSlack()
		producer.Start(sigs, gracefulShutdownWaitingGroup)
	}
}
