package task

import (
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

var metricsIncoming = make(chan string, 1)

// StopPostMetrics stops PostMetrics task sendding signal
func StopPostMetrics() {
	select {
	case metricsIncoming <- "STOP":
	default:
	}
}

var metricsJobsWaitingGroup sync.WaitGroup

//PostMetrics post metrics to Split Events server
func PostMetrics(metricsRecorderAdapter recorder.MetricsRecorder,
	metricsStorageAdapter storage.MetricsStorage,
	metricsRefreshRate int, wg *sync.WaitGroup) {

	wg.Add(1)
	keepLoop := true
	for keepLoop {
		// Increment the WaitGroup counter.
		metricsJobsWaitingGroup.Add(3)
		go sendLatencies(metricsRecorderAdapter, metricsStorageAdapter)
		go sendCounters(metricsRecorderAdapter, metricsStorageAdapter)
		go sendGauges(metricsRecorderAdapter, metricsStorageAdapter)

		metricsJobsWaitingGroup.Wait()

		select {
		case msg := <-metricsIncoming:
			if msg == "STOP" {
				log.Debug.Println("Stopping task: post_metrics")
				keepLoop = false
			}
		case <-time.After(time.Duration(metricsRefreshRate) * time.Second):
		}

	}
	wg.Done()
}

func sendLatencies(metricsRecorderAdapter recorder.MetricsRecorder,
	metricsStorageAdapter storage.MetricsStorage) {

	// Decrement the counter when the goroutine completes.
	defer metricsJobsWaitingGroup.Done()

	latenciesToSend, err := metricsStorageAdapter.RetrieveLatencies()
	if err != nil {
		log.Error.Println(err.Error())
	} else {
		log.Verbose.Println("Latencies to send", latenciesToSend)

		for sdkVersion, latenciesByMachineIP := range latenciesToSend {
			for machineIP, latencies := range latenciesByMachineIP {
				log.Debug.Println("Posting latencies from ", sdkVersion, machineIP)

				var latenciesDataSet []api.LatenciesDTO
				for metricName, latencyValues := range latencies {
					latenciesDataSet = append(latenciesDataSet, api.LatenciesDTO{MetricName: metricName, Latencies: latencyValues})
				}
				metricsRecorderAdapter.PostLatencies(latenciesDataSet, sdkVersion, machineIP)
			}
		}
	}
}

func sendCounters(metricsRecorderAdapter recorder.MetricsRecorder,
	metricsStorageAdapter storage.MetricsStorage) {

	// Decrement the counter when the goroutine completes.
	defer metricsJobsWaitingGroup.Done()

	countersToSend, err := metricsStorageAdapter.RetrieveCounters()
	if err != nil {
		log.Error.Println(err.Error())
	} else {
		log.Verbose.Println("Counters to send", countersToSend)

		for sdkVersion, countersByMachineIP := range countersToSend {
			for machineIP, counters := range countersByMachineIP {
				log.Debug.Println("Posting counters from ", sdkVersion, machineIP)

				var countersDataSet []api.CounterDTO
				for metricName, count := range counters {
					countersDataSet = append(countersDataSet, api.CounterDTO{MetricName: metricName, Count: count})
				}
				metricsRecorderAdapter.PostCounters(countersDataSet, sdkVersion, machineIP)
			}
		}
	}
}

func sendGauges(metricsRecorderAdapter recorder.MetricsRecorder,
	metricsStorageAdapter storage.MetricsStorage) {

	// Decrement the counter when the goroutine completes.
	defer metricsJobsWaitingGroup.Done()

	gaugesToSend, err := metricsStorageAdapter.RetrieveGauges()
	if err != nil {
		log.Error.Println(err.Error())
	} else {
		log.Verbose.Println("Gauges to send", gaugesToSend)

		for sdkVersion, gaugesByMachineIP := range gaugesToSend {
			for machineIP, gauges := range gaugesByMachineIP {
				log.Debug.Println("Posting gauges from ", sdkVersion, machineIP)

				for metricName, value := range gauges {
					gauge := api.GaugeDTO{MetricName: metricName, Gauge: value}
					log.Debug.Println("Posting gauge:", gauge)
					metricsRecorderAdapter.PostGauge(gauge, sdkVersion, machineIP)
				}
			}
		}
	}
}
