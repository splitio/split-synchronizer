package counter

import (
	"errors"
	"sync"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/nethelper"
	"github.com/splitio/go-agent/splitio/recorder"
	"github.com/splitio/go-agent/splitio/stats"
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

type Counter struct {
	counts          map[string]int64
	cmutex          *sync.Mutex
	recorderAdapter recorder.MetricsRecorder
	postRate        int64
}

func (c *Counter) Counts(name string) (int64, error) {
	if _, ok := c.counts[name]; ok {
		return c.counts[name], nil
	}

	return 0, errors.New("Counter not found")
}

func (c *Counter) Increment(name string) {
	c.cmutex.Lock()
	c.counts[name] += 1
	c.cmutex.Unlock()
}

func (c *Counter) Decrement(name string) {
	c.cmutex.Lock()
	c.counts[name] -= 1
	c.cmutex.Unlock()
}

func (c *Counter) IncrementN(name string, inc int64) {
	c.cmutex.Lock()
	c.counts[name] += inc
	c.cmutex.Unlock()
}

func (c *Counter) DecrementN(name string, dec int64) {
	c.cmutex.Lock()
	c.counts[name] -= dec
	c.cmutex.Unlock()
}

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
			stats.SaveCounter(metricName, count)
		}
		// Drop counts
		c.counts = make(map[string]int64)
		c.cmutex.Unlock()

		if len(countersDataSet) > 0 {
			sdkVersion := "goproxy-" + splitio.Version
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
