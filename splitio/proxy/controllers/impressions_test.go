package controllers

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
)

func TestInitialization(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	//Initialize by default
	conf.Initialize()

	conf.Data.Logger.DebugOn = true

	go addImpressionsToBufferWorker(200)

	AddImpressions([]byte("[\"DEMOOO\"]"), "PHP-123", "127.0.0.1")

	//fmt.Println(poolBuffer)
}
