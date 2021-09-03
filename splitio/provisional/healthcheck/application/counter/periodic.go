package counter

import (
	"github.com/splitio/go-toolkit/asynctask"
	"github.com/splitio/go-toolkit/logging"
)

// PeriodicImp description
type PeriodicImp struct {
	ApplicationCounterImp
	errorsCount              int
	maxErrorsAllowedInPeriod int
	task                     *asynctask.AsyncTask
}

// GetErrorsCount description
func (c *PeriodicImp) GetErrorsCount() *int {
	return &c.errorsCount
}

// NotifyEvent description
func (c *PeriodicImp) NotifyEvent() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorsCount++

	if c.errorsCount >= c.maxErrorsAllowedInPeriod {
		c.healthy = false
	}

	c.updateLastHit()
}

// Reset description
func (c *PeriodicImp) Reset(value int) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorsCount = value

	return nil
}

// Start description
func (c *PeriodicImp) Start() {
	c.task.Start()
}

// Stop description
func (c *PeriodicImp) Stop() {
	c.task.Stop(false)
}

// NewCounterPeriodic description
func NewCounterPeriodic(
	config Config,
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
