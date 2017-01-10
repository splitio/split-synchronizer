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
	"github.com/splitio/go-agent/splitio/task"
)

func loadConfiguration() {
	configFile := flag.String("config", "splitio.agent.conf.json", "a configuration file")
	flag.Parse()

	iohelper.Println("Loading config file: ", *configFile)
	conf.Load(*configFile)
}

func getLogWriter(wstdout bool, wfile *os.File) io.Writer {
	if conf.Data.Logger.StdoutOn {
		if wfile != nil {
			return io.MultiWriter(wfile, os.Stdout)
		}
		return io.MultiWriter(os.Stdout)
	}

	if wfile != nil {
		return io.MultiWriter(wfile)
	}

	return ioutil.Discard
}

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

func banner() {
	fmt.Println(splitio.ASCILogo)
	iohelper.Println("Split Software Agent - Version: ", splitio.Version)
}

func startProducer() {
	splitFetcher := fetcher.NewHTTPSplitFetcher(-1)
	splitSorage := storage.NewRedisSplitStorageAdapter(conf.Data.Redis.Host, conf.Data.Redis.Port, conf.Data.Redis.Pass, conf.Data.Redis.Db)
	go task.SplitFetcher(splitFetcher, splitSorage)

}

//------------------------------------------------------------------------------
// MAIN PROGRAM
//------------------------------------------------------------------------------

func init() {
	banner()
	loadConfiguration() // TODO create Initialize function inside conf module
	loadLogger()        // TODO create Initialize function inside log module
	api.Initialize()
}

func main() {

	startProducer()

	//Infinite loop
	for {
		time.Sleep(15 * time.Second)
	}
}
