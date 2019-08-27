package task

import (
	"sync"
	"time"

	"fmt"
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
	metricsPostRate int, wg *sync.WaitGroup) {

	wg.Add(1)
	keepLoop := true
	for keepLoop {
		// Increment the WaitGroup counter.
		metricsJobsWaitingGroup.Add(3)
		go sendLatencies(metricsRecorderAdapter, metricsStorageAdapter)
		go sendCounters(metricsRecorderAdapter, metricsStorageAdapter)
		go sendGauges(metricsRecorderAdapter, metricsStorageAdapter)
		fmt.Println("Tareas de metricas corriendo")
		metricsJobsWaitingGroup.Wait()

		select {
		case msg := <-metricsIncoming:
			if msg == "STOP" {
				log.Debug.Println("Stopping task: post_metrics")
				keepLoop = false
			}
		case <-time.After(time.Duration(metricsPostRate) * time.Second):
		}

	}
	wg.Done()
}

func sendLatencies(metricsRecorderAdapter recorder.MetricsRecorder,
	metricsStorageAdapter storage.MetricsStorage) {

	// Decrement the counter when the goroutine completes.
	defer metricsJobsWaitingGroup.Done()

	latenciesToSend, err := metricsStorageAdapter.RetrieveLatencies()
	fmt.Println("Latencias sacadas de redis: ", latenciesToSend)
	if err != nil {
		fmt.Println("Error", err.Error())
		log.Error.Println(err.Error())
	} else {
		log.Verbose.Println("Latencies to send", latenciesToSend)

		latenciesToSend.ForEach(func(sdk string, ip string, latencies map[string][]int64) {
			latenciesDataSet := make([]api.LatenciesDTO, 0)
			for name, buckets := range latencies {
				latenciesDataSet = append(latenciesDataSet, api.LatenciesDTO{MetricName: name, Latencies: buckets})
			}
			metricsRecorderAdapter.PostLatencies(latenciesDataSet, sdk, ip)
		})
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

		countersToSend.ForEach(func(sdk string, ip string, counters map[string]int64) {
			countersDataSet := make([]api.CounterDTO, 0)
			for metricName, count := range counters {
				countersDataSet = append(countersDataSet, api.CounterDTO{MetricName: metricName, Count: count})
			}
			metricsRecorderAdapter.PostCounters(countersDataSet, sdk, ip)
		})
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
		gaugesToSend.ForEach(func(sdk string, ip string, metricName string, value float64) {
			log.Debug.Println("Posting gauge:", metricName, value)
			metricsRecorderAdapter.PostGauge(api.GaugeDTO{MetricName: metricName, Gauge: value}, sdk, ip)
		})
	}
}
