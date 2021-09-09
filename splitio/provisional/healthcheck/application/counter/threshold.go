package counter

import (
	"fmt"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

// ThresholdImp description
type ThresholdImp struct {
	ApplicationCounterImp
	cancel chan struct{}
	reset  chan struct{}
}

// GetErrorCount get error count
func (c *ThresholdImp) GetErrorCount() *int {
	// no-op
	return nil
}

// NotifyEvent reset the timer
func (c *ThresholdImp) NotifyEvent() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.reset <- struct{}{}
	c.updateLastHit()
}

// Reset the threshold value
func (c *ThresholdImp) Reset(newThreshold int) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if newThreshold <= 0 {
		return fmt.Errorf("refreshTreshold should be > 0")
	}

	c.period = newThreshold
	c.reset <- struct{}{}

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

	go func() {
		timer := time.NewTimer(time.Duration(c.period) * time.Second)
		c.running = true
		for c.running {
			select {
			case <-timer.C:
				c.healthy = false
				c.running = false
			case <-c.reset:
				timer.Reset(time.Duration(c.period) * time.Second)
			case <-c.cancel:
				c.running = false
			}
		}
	}()
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

// NewCounterThresholdImp create Threshold counter
func NewCounterThresholdImp(
	config Config,
	logger logging.LoggerInterface,
) *ThresholdImp {
	return &ThresholdImp{
		ApplicationCounterImp: *NewApplicationCounterImp(config.Name, config.CounterType, config.Period, config.Severity, logger),
		cancel:                make(chan struct{}, 1),
		reset:                 make(chan struct{}, 1),
	}
}
