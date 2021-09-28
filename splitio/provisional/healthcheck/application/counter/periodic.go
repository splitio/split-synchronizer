package counter

import (
	"fmt"
	"sync"

	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

// PeriodicImp periodic counter struct
type PeriodicImp struct {
	applicationCounterImp
	maxErrorsAllowedInPeriod int
	goroutineFunc            func(c ApplicationCounterInterface)
	task                     *asynctask.AsyncTask
}

// NotifyEvent increase errorCount and check the health
func (c *PeriodicImp) NotifyEvent() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorCount++

	if c.errorCount >= c.maxErrorsAllowedInPeriod {
		c.healthy = false
	} else {
		c.healthy = true
	}

	c.updateLastHit()
}

// Reset errorCount
func (c *PeriodicImp) Reset(value int) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorCount = value

	return nil
}

// Start counter
func (c *PeriodicImp) Start() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.running {
		c.logger.Debug(fmt.Sprintf("%s counter is alredy running.", c.name))
		return
	}

	c.task.Start()
	c.running = true

	go func() {
		for c.running {
			c.goroutineFunc(c)
		}
	}()
}

// Stop counter
func (c *PeriodicImp) Stop() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.running {
		c.logger.Debug(fmt.Sprintf("%s counter is alredy stopped.", c.name))
		return
	}

	c.task.Stop(false)
	c.running = false
}

// NewPeriodicCounter create new periodic counter
func NewPeriodicCounter(
	config *Config,
	logger logging.LoggerInterface,
) *PeriodicImp {
	counter := &PeriodicImp{
		applicationCounterImp: applicationCounterImp{
			name:        config.Name,
			lock:        sync.RWMutex{},
			logger:      logger,
			healthy:     true,
			running:     false,
			counterType: config.CounterType,
			period:      config.Period,
			severity:    config.Severity,
			monitorType: config.MonitorType,
		},
		maxErrorsAllowedInPeriod: config.MaxErrorsAllowedInPeriod,
		goroutineFunc:            config.GoroutineFunc,
	}

	counter.task = asynctask.NewAsyncTask(config.Name, func(l logging.LoggerInterface) error {
		return config.TaskFunc(l, counter)
	}, counter.period, nil, nil, logger)

	return counter
}
