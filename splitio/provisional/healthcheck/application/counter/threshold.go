package counter

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

// ThresholdImp description
type ThresholdImp struct {
	applicationCounterImp
	cancel chan struct{}
	reset  chan struct{}
}

// NotifyEvent reset the timer
func (c *ThresholdImp) NotifyEvent() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.running {
		c.logger.Debug(fmt.Sprintf("%s counter  is not running.", c.name))
		return
	}

	c.reset <- struct{}{}
	c.updateLastHit()

	c.logger.Debug("NotifyEvent threshold counter.")
}

// Reset the threshold value
func (c *ThresholdImp) Reset(newThreshold int) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.running {
		c.logger.Debug(fmt.Sprintf("%s counter is not running.", c.name))
		return nil
	}

	if newThreshold <= 0 {
		return fmt.Errorf("refreshTreshold should be > 0")
	}

	c.period = newThreshold
	c.reset <- struct{}{}

	c.logger.Debug("Reset treshold counter.")

	return nil
}

// Start counter and timer
func (c *ThresholdImp) Start() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.running {
		c.logger.Debug(fmt.Sprintf("%s counter is alredy running.", c.name))
		return
	}
	c.running = true

	go func() {
		timer := time.NewTimer(time.Duration(c.period) * time.Second)
		defer timer.Stop()
		defer func() { c.lock.Lock(); c.running = false; c.lock.Unlock() }()
		for {
			select {
			case <-timer.C:
				c.lock.Lock()
				c.healthy = false
				c.lock.Unlock()
				return
			case <-c.reset:
				timer.Reset(time.Duration(c.period) * time.Second)
			case <-c.cancel:
				return
			}
		}
	}()

	c.logger.Debug(fmt.Sprintf("%s threshold counter started.", c.name))
}

// Stop counter
func (c *ThresholdImp) Stop() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.running {
		c.logger.Debug(fmt.Sprintf("%s counter is alredy stopped.", c.name))
		return
	}

	c.cancel <- struct{}{}
}

// NewThresholdCounter create Threshold counter
func NewThresholdCounter(
	config *Config,
	logger logging.LoggerInterface,
) *ThresholdImp {
	return &ThresholdImp{
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
		cancel: make(chan struct{}, 1),
		reset:  make(chan struct{}, 1),
	}
}
