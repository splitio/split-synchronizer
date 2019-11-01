// Package latency implements functions to track latencies
package latency

import (
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/nethelper"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/stats"
)

// NewLatency returns a Latency instance
func NewLatency() *Latency {
	latency := &Latency{latencies: make(map[string][]int64),
		lmutex:          &sync.Mutex{},
		recorderAdapter: recorder.MetricsHTTPRecorder{},
		postRate:        60}

	go latency.PostLatenciesWorker(stats.SaveLatency)

	return latency
}

// NewLatencyBucket returns a LatencyBucket instance
func NewLatencyBucket() *LatencyBucket {
	latency := &Latency{latencies: make(map[string][]int64),
		lmutex:          &sync.Mutex{},
		recorderAdapter: recorder.MetricsHTTPRecorder{},
		postRate:        60}

	go latency.PostLatenciesWorker(stats.SaveLatencyBkt)

	latencyBkt := &LatencyBucket{latency}

	return latencyBkt
}

//------------------------------------------------------------------------------
//Latency
//------------------------------------------------------------------------------

// Latency struct to track http latencies
type Latency struct {
	latencies       map[string][]int64
	lmutex          *sync.Mutex
	recorderAdapter recorder.MetricsRecorder
	postRate        int64
}

// StartMeasuringLatency return a checkpoint number in nanoseconds
func (l *Latency) StartMeasuringLatency() int64 {
	return time.Now().UnixNano()
}

// calculateLatency given the checkpoint number returns the elapsed microseconds
func (l *Latency) calculateLatency(timeStart int64) int64 {
	timeEnd := time.Now().UnixNano()
	return int64(float64(timeEnd-timeStart) * 0.001)
}

// RegisterLatency regists
func (l *Latency) RegisterLatency(name string, startCheckpoint int64) {
	latency := l.calculateLatency(startCheckpoint)
	//bucket := l.getBucketForLatencyMicros(latency)
	l.lmutex.Lock()
	if l.latencies[name] == nil {
		//l.latencies[name] = make([]int64, len(buckets))
		l.latencies[name] = make([]int64, 0)
	}

	//l.latencies[name][bucket] += 1
	l.latencies[name] = append(l.latencies[name], latency)
	l.lmutex.Unlock()
}

// PostLatenciesWorker posts latencies
func (l *Latency) PostLatenciesWorker(f stats.LatencyStorageAddFunc) {
	for {
		select {
		case <-time.After(time.Second * time.Duration(l.postRate)):
			log.Debug.Println("Posting go proxy latencies")
		}

		l.lmutex.Lock()

		var latenciesDataSet []api.LatenciesDTO
		for metricName, latencyValues := range l.latencies {
			latenciesDataSet = append(latenciesDataSet, api.LatenciesDTO{MetricName: metricName, Latencies: latencyValues})
			f(metricName, latencyValues)
		}
		//Dropping latencies
		l.latencies = make(map[string][]int64)

		l.lmutex.Unlock()
		sdkVersion := appcontext.VersionHeader()
		if len(latenciesDataSet) > 0 {
			errp := l.recorderAdapter.PostLatencies(latenciesDataSet, sdkVersion, nethelper.ExternalIP())
			if errp != nil {
				log.Error.Println("Go-proxy latencies worker:", errp)
			}
		}
	}
}

//------------------------------------------------------------------------------
//Latency Bucket
//------------------------------------------------------------------------------

const maxLatency = 7481828

var buckets = []int64{1000, 1500, 2250, 3375, 5063, 7594, 11391, 17086, 25629, 38443, 57665, 86498, 129746, 194620, 291929, 437894, 656841, 985261, 1477892, 2216838, 3325257, 4987885, 7481828}

// LatencyBucket represents latencies grouped by microseconds time-frame
type LatencyBucket struct {
	*Latency
}

// getBucketForLatencyMicros returns the bucket number to increment latency
func (l *LatencyBucket) getBucketForLatencyMicros(latency int64) int {
	for k, v := range buckets {
		if latency <= v {
			return k
		}
	}
	return len(buckets) - 1
}

// RegisterLatency regists
func (l *LatencyBucket) RegisterLatency(name string, startCheckpoint int64) {
	latency := l.calculateLatency(startCheckpoint)
	bucket := l.getBucketForLatencyMicros(latency)
	l.lmutex.Lock()
	if l.latencies[name] == nil {
		l.latencies[name] = make([]int64, len(buckets))
	}
	l.latencies[name][bucket] += 1
	l.lmutex.Unlock()
}
