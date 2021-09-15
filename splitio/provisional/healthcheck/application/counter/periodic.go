package counter

import (
	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

// PeriodicImp periodic counter struct
type PeriodicImp struct {
	ApplicationCounterImp
	maxErrorsAllowedInPeriod int
	task                     *asynctask.AsyncTask
}

// NotifyEvent increase errorCount and check the health
func (c *PeriodicImp) NotifyEvent() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorCount++

	if c.errorCount >= c.maxErrorsAllowedInPeriod {
		c.healthy = false
	}

	c.UpdateLastHit()
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
	c.task.Start()
}

// Stop counter
func (c *PeriodicImp) Stop() {
	c.task.Stop(false)
}

// NewCounterPeriodic create new periodic counter
func NewCounterPeriodic(
	config *hcCommon.Config,
	logger logging.LoggerInterface,
) *PeriodicImp {
	counter := &PeriodicImp{
		ApplicationCounterImp:    *NewApplicationCounterImp(config.Name, config.CounterType, config.Period, config.Severity, logger),
		maxErrorsAllowedInPeriod: config.MaxErrorsAllowedInPeriod,
	}

	counter.task = asynctask.NewAsyncTask(config.Name, func(l logging.LoggerInterface) error {
		return config.TaskFunc(l, counter)
	}, counter.period, nil, nil, logger)

	return counter
}
