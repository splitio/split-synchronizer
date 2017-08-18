package counter

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/stats"
)

func TestCounter(t *testing.T) {

	counterA := "COUNTER_A"
	counterB := "COUNTER_B"

	var expectedA int64 = 1 + 7 - 1 - 5
	var expectedB int64 = 1 + 5 - 1 - 15

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
		var countersInPost []api.CounterDTO
		err := json.Unmarshal(rBody, &countersInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if countersInPost[0].MetricName != counterA ||
			countersInPost[0].Count != expectedA ||
			countersInPost[1].MetricName != counterB ||
			countersInPost[1].Count != expectedB {
			t.Error("Posted counters arrived mal-formed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()
	stats.Initialize()
	// Counter Code
	counter := NewCounter()
	counter.postRate = 5

	counter.Increment(counterA)
	counter.IncrementN(counterA, 7)
	counter.Decrement(counterA)
	counter.DecrementN(counterA, 5)

	counter.Increment(counterB)
	counter.IncrementN(counterB, 5)
	counter.Decrement(counterB)
	counter.DecrementN(counterB, 15)

	// testing counterA
	if counts, err := counter.Counts(counterA); err != nil {
		t.Error(err)
	} else if counts != expectedA {
		t.Error(fmt.Errorf("Invalid count: Expected %d given %d", expectedA, counts))
	}

	// testing counterB
	if counts, err := counter.Counts(counterB); err != nil {
		t.Error(err)
	} else if counts != expectedB {
		t.Error(fmt.Errorf("Invalid count: Expected %d given %d", expectedB, counts))
	}

	//Delaying test to let PostCounterWorker timeout do its work!
	time.Sleep(time.Duration(10) * time.Second)

}
