package counter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/conf"

	"github.com/splitio/split-synchronizer/appcontext"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/stats"
)

func getCounterByName(countersInPost []api.CounterDTO, name string) (*api.CounterDTO, error) {
	for _, c := range countersInPost {
		if c.MetricName == name {
			return &c, nil
		}
	}
	return nil, errors.New("Counter not found")
}

func TestCounter(t *testing.T) {

	counterA := "COUNTER_A"
	counterB := "COUNTER_B"

	conf.Initialize()
	conf.Data.IPAddressesEnabled = false

	var expectedA int64 = 1 + 7 - 1 - 5
	var expectedB int64 = 1 + 5 - 1 - 15

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		sdkVersion := r.Header.Get("SplitSDKVersion")
		sdkMachine := r.Header.Get("SplitSDKMachineIP")

		if sdkVersion != "SplitSyncProducerMode-"+splitio.Version {
			t.Error("SDK Version HEADER not match")
		}

		if sdkMachine != "" {
			t.Error("Header should not be present")
		}

		sdkMachineName := r.Header.Get("SplitSDKMachineName")
		if sdkMachineName != "" {
			t.Error("Header should not be present")
		}

		rBody, _ := ioutil.ReadAll(r.Body)
		var countersInPost []api.CounterDTO
		err := json.Unmarshal(rBody, &countersInPost)
		if err != nil {
			t.Error(err)
			return
		}

		counterForMetricA, err := getCounterByName(countersInPost, counterA)
		if err != nil {
			t.Error("Did not recieve counters for metric A")
		}

		counterForMetricB, err := getCounterByName(countersInPost, counterB)
		if err != nil {
			t.Error("Did not recieve counters for metric B")
		}

		if counterForMetricA.Count != expectedA {
			t.Errorf("Expected count to be %d, got %d", expectedA, counterForMetricA.Count)
		}

		if counterForMetricB.Count != expectedB {
			t.Errorf("Expected count to be %d, got %d", expectedB, counterForMetricB.Count)
		}
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()
	stats.Initialize()
	appcontext.Initialize(appcontext.ProducerMode)
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
	time.Sleep(time.Duration(20) * time.Second)

}
