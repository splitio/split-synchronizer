// Package counter implements metrics counters
package counter

import (
	"errors"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/nethelper"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/stats"
)

// NewCounter returns a Counter instance
func NewCounter() *Counter {
	counter := &Counter{counts: make(map[string]int64),
		cmutex:          &sync.Mutex{},
		recorderAdapter: recorder.MetricsHTTPRecorder{},
		postRate:        60}

	go counter.PostCounterWorker()

	return counter
}

// Counter atomic counter
type Counter struct {
	counts          map[string]int64
	cmutex          *sync.Mutex
	recorderAdapter recorder.MetricsRecorder
	postRate        int64
}

// Counts returns the total count for a given metric name
func (c *Counter) Counts(name string) (int64, error) {
	if _, ok := c.counts[name]; ok {
		return c.counts[name], nil
	}

	return 0, errors.New("Counter not found")
}

// Increment a counter +1 for the given metric name
func (c *Counter) Increment(name string) {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	c.counts[name]++
}

// Decrement a counter -1 for the given metric name
func (c *Counter) Decrement(name string) {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	c.counts[name]--
}

// IncrementN a counter +N for the given metric name
func (c *Counter) IncrementN(name string, inc int64) {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	c.counts[name] += inc
}

// DecrementN a counter -N for the given metric name
func (c *Counter) DecrementN(name string, dec int64) {
	c.cmutex.Lock()
	c.counts[name] -= dec
	c.cmutex.Unlock()
}

// PostCounterWorker post counter metrics
func (c *Counter) PostCounterWorker() {
	for {
		select {
		case <-time.After(time.Second * time.Duration(c.postRate)):
			log.Debug.Println("Posting go proxy counters")
		}

		c.cmutex.Lock()
		var countersDataSet []api.CounterDTO
		for metricName, count := range c.counts {
			countersDataSet = append(countersDataSet, api.CounterDTO{MetricName: metricName, Count: count})
			//stats.SaveCounter(metricName, count)
		}
		// Drop counts
		c.counts = make(map[string]int64)
		c.cmutex.Unlock()

		if len(countersDataSet) > 0 {
			sdkVersion := appcontext.VersionHeader()
			machineIP, err := nethelper.ExternalIP()
			if err != nil {
				machineIP = "unknown"
			}
			errc := c.recorderAdapter.PostCounters(countersDataSet, sdkVersion, machineIP)
			if errc != nil {
				log.Error.Println(errc)
			}
		}
	}
}

//------------------------------------------------------------------------------
// LOCAL COUNTER
//------------------------------------------------------------------------------

// NewLocalCounter returns a Local Counter instance
func NewLocalCounter() *LocalCounter {
	counter := &Counter{counts: make(map[string]int64),
		cmutex:          &sync.Mutex{},
		recorderAdapter: recorder.MetricsHTTPRecorder{},
		postRate:        60}

	localCounter := &LocalCounter{counter}

	go localCounter.PostCounterWorker()

	return localCounter
}

// LocalCounter struc to count metrics locally and not post to Split servers
type LocalCounter struct {
	*Counter
}

// PostCounterWorker post metrics to local storage
func (c *LocalCounter) PostCounterWorker() {

	for {
		select {
		case <-time.After(time.Second * time.Duration(c.postRate)):
			log.Debug.Println("Posting LOCAL proxy counters")
		}

		c.cmutex.Lock()
		for metricName, count := range c.counts {
			stats.SaveCounter(metricName, count)
		}
		// Drop counts
		c.counts = make(map[string]int64)
		c.cmutex.Unlock()
	}

}
