package latency

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio"
	"github.com/splitio/go-agent/splitio/api"
)

func TestLatency(t *testing.T) {

	latencyA := "LATENCY_A"

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "goproxy-"+splitio.Version {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine == "" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName == "" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		//fmt.Println(string(rBody))
		var latenciesInPost []api.LatenciesDTO
		err := json.Unmarshal(rBody, &latenciesInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if latenciesInPost[0].MetricName != latencyA ||
			!(latenciesInPost[0].Latencies[0] > 1) ||
			!(latenciesInPost[0].Latencies[1] > 1) {
			t.Error("Posted latencies arrived mal-formed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	latency := NewLatency()
	latency.postRate = 2

	start := latency.StartMeasuringLatency()
	time.Sleep(time.Duration(10) * time.Microsecond)
	latency.RegisterLatency(latencyA, start)

	start = latency.StartMeasuringLatency()
	time.Sleep(time.Duration(11391) * time.Microsecond)
	latency.RegisterLatency(latencyA, start)

	/*if latency.latencies[latencyA][0] != 1 {
		t.Error("Bucket invalid")
	}
	if latency.latencies[latencyA][7] != 1 {
		t.Error("Bucket invalid")
	}*/

	if len(latency.latencies[latencyA]) != 2 &&
		!(latency.latencies[latencyA][0] > 0) &&
		!(latency.latencies[latencyA][1] > 0) {
		t.Error("Unregistered latency")
	}

	//Delaying test to let PostLatenciesWorker timeout do its work!
	time.Sleep(time.Duration(3) * time.Second)
}

func TestLatencyBucket(t *testing.T) {

	latencyA := "LATENCY_BKT"

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "goproxy-"+splitio.Version {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine == "" {
			t.Error("SDK Machine HEADER not match")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName == "" {
			t.Error("SDK Machine Name HEADER not match", sdkMachineName)
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		//fmt.Println(string(rBody))
		var latenciesInPost []api.LatenciesDTO
		err := json.Unmarshal(rBody, &latenciesInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if latenciesInPost[0].MetricName != latencyA ||
			!(latenciesInPost[0].Latencies[0] == 1) ||
			!(latenciesInPost[0].Latencies[7] == 1) {
			t.Error("Posted latencies arrived mal-formed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	latency := NewLatencyBucket()
	latency.postRate = 2

	start := latency.StartMeasuringLatency()
	time.Sleep(time.Duration(10) * time.Microsecond)
	latency.RegisterLatency(latencyA, start)

	start = latency.StartMeasuringLatency()
	time.Sleep(time.Duration(11391) * time.Microsecond)
	latency.RegisterLatency(latencyA, start)

	if latency.latencies[latencyA][0] != 1 {
		t.Error("Bucket invalid")
	}
	if latency.latencies[latencyA][7] != 1 {
		t.Error("Bucket invalid")
	}

	//Delaying test to let PostLatenciesWorker timeout do its work!
	time.Sleep(time.Duration(3) * time.Second)
}
