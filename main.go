// main.go
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/iohelper"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/fetcher"
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
	iohelper.Println("Loading config file: ", *configFile)
	conf.Load(*configFile)
}

func getLogWriter(wstdout bool, wfile *os.File) io.Writer {
	if conf.Data.Logger.StdoutOn {
		if wfile != nil {
			slack := &log.SlackWriter{WebHookURL: conf.Data.Logger.SlackWebhookURL, Channel: conf.Data.Logger.SlackChannel}
			return io.MultiWriter(wfile, os.Stdout, slack)
		}
		return io.MultiWriter(os.Stdout)
	}

	if wfile != nil {
		return io.MultiWriter(wfile)
	}

	return ioutil.Discard
}

// TODO add SlackWriter as log handler for Errors
func loadLogger() {
	var multi io.Writer

	if len(conf.Data.Logger.File) > 3 {
		file, err := os.OpenFile(conf.Data.Logger.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if errors.IsError(err) {
			iohelper.PrintlnError(err, "Failed to open log file ")
			multi = getLogWriter(conf.Data.Logger.StdoutOn, nil)
		} else {
			iohelper.Println("Log file: ", file.Name())
			multi = getLogWriter(conf.Data.Logger.StdoutOn, file)
		}
	} else {
		iohelper.Println("Initializing without log file.")
		multi = getLogWriter(conf.Data.Logger.StdoutOn, nil)
	}

	log.Initialize(multi, conf.Data.Logger.DebugOn, conf.Data.Logger.VerboseOn)

}

func startProducer() {

	//redisClient := redis.NewInstance(conf.Data.Redis.Host, conf.Data.Redis.Port,
	//	conf.Data.Redis.Pass, conf.Data.Redis.Db)

	//splitFetcher := splitFetcherFactory()
	//splitSorage := splitStorageFactory()
	//go task.FetchSplits(splitFetcher, splitSorage)

	segmentFetcher := segmentFetcherFactory()
	segmentStorage := segmentStorageFactory()
	go task.FetchSegments(segmentFetcher, segmentStorage)

}

func splitFetcherFactory() fetcher.SplitFetcher {
	return fetcher.NewHTTPSplitFetcher(-1)
}

func splitStorageFactory() storage.SplitStorage {
	return redis.NewSplitStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}

func segmentFetcherFactory() fetcher.SegmentFetcherFactory {
	return fetcher.SegmentFetcherFactory{}
}

func segmentStorageFactory() storage.SegmentStorage {
	return redis.NewSegmentStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
}
