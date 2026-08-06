package counter

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	toolkitsync "github.com/splitio/go-toolkit/v5/sync"
)

// ThresholdCounterInterface application counter interface
type ThresholdCounterInterface interface {
	IsHealthy() HealthyResult
	NotifyHit()
	ResetThreshold(value int) error
	Start()
	Stop()
}

// ThresholdImp description
type ThresholdImp struct {
	applicationCounterImp
	cancel chan struct{}
	reset  chan struct{}
}

// ThresholdConfig config struct
type ThresholdConfig struct {
	Name     string
	Period   int
	Severity int
}

// NotifyHit reset the timer
func (c *ThresholdImp) NotifyHit() {
	if !c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter  is not running.", c.name))
		return
	}

	c.reset <- struct{}{}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.updateLastHit()

	c.logger.Debug(fmt.Sprintf("event received for counter '%s'", c.name))
}

// ResetThreshold the threshold value
func (c *ThresholdImp) ResetThreshold(newThreshold int) error {
	if !c.running.IsSet() {
		c.logger.Warning(fmt.Sprintf("%s counter is not running.", c.name))
		return nil
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if newThreshold <= 0 {
		return fmt.Errorf("refreshTreshold should be > 0")
	}

	c.period = newThreshold
	c.reset <- struct{}{}

	c.logger.Debug(fmt.Sprintf("updated threshold for counter '%s' to %d seconds", c.name, newThreshold))

	return nil
}

// IsHealthy return the counter health
func (c *ThresholdImp) IsHealthy() HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return HealthyResult{
		Name:     c.name,
		Healthy:  c.healthy,
		Severity: c.severity,
		LastHit:  c.lastHit,
	}
}

// Start counter and timer
func (c *ThresholdImp) Start() {
	if c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter is already running.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.running.Set()

	go func() {
		c.lock.Lock()
		timer := time.NewTimer(time.Duration(c.period) * time.Second)
		c.lock.Unlock()

		defer timer.Stop()
		defer c.running.Unset()
		for {
			select {
			case <-timer.C:
				c.lock.Lock()
				c.logger.Error(fmt.Sprintf("counter '%s' has timed out with tolerance=%ds", c.name, c.period))
				c.healthy = false
				c.lock.Unlock()
				return
			case <-c.reset:
				c.lock.Lock()
				timer.Reset(time.Duration(c.period) * time.Second)
				c.lock.Unlock()
			case <-c.cancel:
				return
			}
		}
	}()

	c.logger.Debug(fmt.Sprintf("%s threshold counter started.", c.name))
}

// Stop counter
func (c *ThresholdImp) Stop() {
	if !c.running.IsSet() {
		c.logger.Debug(fmt.Sprintf("%s counter is already stopped.", c.name))
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.cancel <- struct{}{}
}

// NewThresholdCounter create Threshold counter
func NewThresholdCounter(
	config ThresholdConfig,
	logger logging.LoggerInterface,
) *ThresholdImp {
	return &ThresholdImp{
		applicationCounterImp: applicationCounterImp{
			name:     config.Name,
			lock:     sync.RWMutex{},
			logger:   logger,
			healthy:  true,
			running:  *toolkitsync.NewAtomicBool(false),
			period:   config.Period,
			severity: config.Severity,
		},
		cancel: make(chan struct{}, 1),
		reset:  make(chan struct{}, 1),
	}
}

// DefaultThresholdConfig new config with default values
func DefaultThresholdConfig(
	name string,
) ThresholdConfig {
	return ThresholdConfig{
		Name:     name,
		Period:   3600,
		Severity: Critical,
	}
}
