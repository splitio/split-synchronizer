package counter

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
	toolkitsync "github.com/splitio/go-toolkit/v5/sync"
)

// PeriodicCounterInterface application counter interface
type PeriodicCounterInterface interface {
	IsHealthy() HealthyResult
	NotifyError()
	Start()
	Stop()
}

// PeriodicConfig config struct
type PeriodicConfig struct {
	Name                     string
	Period                   int
	Severity                 int
	ValidationFunc           func(c PeriodicCounterInterface)
	ValidationFuncPeriod     int
	MaxErrorsAllowedInPeriod int
}

// PeriodicImp periodic counter struct
type PeriodicImp struct {
	applicationCounterImp
	errorCount               int
	maxErrorsAllowedInPeriod int
	validationFunc           func(c PeriodicCounterInterface)
	validationFuncPeriod     int
	task                     *asynctask.AsyncTask
}

func (c *PeriodicImp) resetErrorCount() {
	if !c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter  is not running.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorCount = 0

	return
}

// NotifyError increase errorCount and check the health
func (c *PeriodicImp) NotifyError() {
	if !c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter  is not running.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.errorCount++
	c.healthy = c.maxErrorsAllowedInPeriod > c.errorCount
	c.updateLastHit()

	c.logger.Debug("NotifyEvent periodic counter.")
}

// IsHealthy return the counter health
func (c *PeriodicImp) IsHealthy() HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return HealthyResult{
		Name:       c.name,
		Healthy:    c.healthy,
		Severity:   c.severity,
		LastHit:    c.lastHit,
		ErrorCount: c.errorCount,
	}
}

// Start counter
func (c *PeriodicImp) Start() {
	if c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s periodic counter is already running.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.task.Start()
	c.running.Set()

	go func() {
		for c.running.IsSet() {
			time.Sleep(time.Duration(c.validationFuncPeriod) * time.Second)
			c.validationFunc(c)
			c.lock.Lock()
			c.updateLastHit()
			c.lock.Unlock()
		}
	}()

	c.logger.Debug(fmt.Sprintf("%s periodic counter started.", c.name))
}

// Stop counter
func (c *PeriodicImp) Stop() {
	if !c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter is alredy stopped.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.task.Stop(false)
	c.running.Unset()
}

// NewPeriodicCounter create new periodic counter
func NewPeriodicCounter(
	config PeriodicConfig,
	logger logging.LoggerInterface,
) *PeriodicImp {
	counter := &PeriodicImp{
		applicationCounterImp: applicationCounterImp{
			name:     config.Name,
			lock:     sync.RWMutex{},
			logger:   logger,
			healthy:  true,
			running:  *toolkitsync.NewAtomicBool(false),
			period:   config.Period,
			severity: config.Severity,
		},
		maxErrorsAllowedInPeriod: config.MaxErrorsAllowedInPeriod,
		validationFunc:           config.ValidationFunc,
		validationFuncPeriod:     config.ValidationFuncPeriod,
	}

	counter.task = asynctask.NewAsyncTask(config.Name, func(l logging.LoggerInterface) error {
		counter.resetErrorCount()
		return nil
	}, counter.period, nil, nil, logger)

	return counter
}

// DefaultPeriodicConfig new config with default values
func DefaultPeriodicConfig(
	name string,
) PeriodicConfig {
	return PeriodicConfig{
		Name:     name,
		Period:   3600,
		Severity: Critical,
	}
}
