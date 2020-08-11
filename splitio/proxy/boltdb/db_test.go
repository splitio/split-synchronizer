package boltdb

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

func before() {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	//Initialize by default
	conf.Initialize()

	conf.Data.Logger.DebugOn = true
}

func TestInitialize(t *testing.T) {
	before()
	Initialize(InMemoryMode, nil)

	err := DBB.Close()
	if err != nil {
		t.Error(err)
	}

}
