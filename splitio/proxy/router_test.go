// +build !race

package proxy

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/common"
)

func TestRouterWithoutHeaders(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	conf.Initialize()
	proxyOptions := &Options{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   []string{"one", "two"},
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
	}

	go Run(proxyOptions)

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://localhost:3000/api/auth", nil)
	if err != nil {
		t.Error(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Error("It should return err")
	}
}

func TestRouterWrongApikey(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	proxyOptions := &Options{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   []string{"one", "two"},
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
		httpClients:               common.HTTPClients{},
		segmentStorage:            nil,
		splitStorage:              nil,
	}

	go Run(proxyOptions)

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://localhost:3000/api/auth", nil)
	if err != nil {
		t.Error(err)
	}

	req.Header.Set("Authorization", "Bearer wrong")
	resp, err := client.Do(req)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Error("It should return err")
	}
}

func TestRouterOk(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	conf.Initialize()
	proxyOptions := &Options{
		Port:                      ":" + strconv.Itoa(conf.Data.Proxy.Port),
		APIKeys:                   []string{"one", "two"},
		AdminPort:                 conf.Data.Proxy.AdminPort,
		AdminUsername:             conf.Data.Proxy.AdminUsername,
		AdminPassword:             conf.Data.Proxy.AdminPassword,
		DebugOn:                   conf.Data.Logger.DebugOn,
		ImpressionListenerEnabled: conf.Data.ImpressionListener.Endpoint != "",
		httpClients:               common.HTTPClients{},
		segmentStorage:            nil,
		splitStorage:              nil,
	}

	go Run(proxyOptions)

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://localhost:3000/api/auth", nil)
	if err != nil {
		t.Error(err)
	}

	req.Header.Set("Authorization", "Bearer one")
	resp, err := client.Do(req)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error("It should not return err")
	}
}
