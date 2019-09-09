// Package task contains all agent tasks
package task

import (
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

/* MetricsRecorder for testing*/
type testMetricsRecorder struct{}

func (r testMetricsRecorder) PostLatencies(latencies []api.LatenciesDTO, sdkVersion string, machineIP string) error {
	return nil
}
func (r testMetricsRecorder) PostCounters(counters []api.CounterDTO, sdkVersion string, machineIP string) error {
	return nil
}
func (r testMetricsRecorder) PostGauge(gauge api.GaugeDTO, sdkVersion string, machineIP string) error {
	return nil
}

/* MetricsStorage for testing */
type testMetricsStorage struct{}

//returns [sdkNameAndVersion][machineIP][metricName] = int64
func (r testMetricsStorage) RetrieveCounters() (*storage.CounterDataBulk, error) {
	toReturn := storage.NewCounterDataBulk()
	toReturn.PutCounter("test-2.0", "127.0.0.1", "some_counter", 124)
	return toReturn, nil
}

//returns [sdkNameAndVersion][machineIP][metricName] = [0,0,0,0,0,0,0,0,0,0,0 ... ]
func (r testMetricsStorage) RetrieveLatencies() (*storage.LatencyDataBulk, error) {
	toReturn := storage.NewLatencyDataBulk()
	toReturn.PutLatency("test-2.0", "127.0.0.1", "some_counter", 1, 111)
	toReturn.PutLatency("test-2.0", "127.0.0.1", "some_counter", 2, 222)
	return toReturn, nil
}

//returns [sdkNameAndVersion][machineIP][metricName] = float64
func (r testMetricsStorage) RetrieveGauges() (*storage.GaugeDataBulk, error) {
	toReturn := storage.NewGaugeDataBulk()
	toReturn.PutGauge("test-2.0", "127.0.0.1", "some_gauge", 1.23)
	return toReturn, nil
}

func TestPostMetrics(t *testing.T) {

	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	metricsRecorderAdapter := testMetricsRecorder{}
	metricsStorageAdapter := testMetricsStorage{}

	// Increment the WaitGroup counter.
	metricsJobsWaitingGroup.Add(3)

	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		sendLatencies(metricsRecorderAdapter, metricsStorageAdapter)
		sendCounters(metricsRecorderAdapter, metricsStorageAdapter)
		sendGauges(metricsRecorderAdapter, metricsStorageAdapter)
	}()
}
