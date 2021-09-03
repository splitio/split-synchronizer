package counter

import (
	"fmt"
	"time"

	"github.com/splitio/go-toolkit/asynctask"
	"github.com/splitio/go-toolkit/logging"
)

// SecuencialImp description
type SecuencialImp struct {
	BaseCounterImp
	maxErrorsAllowed   int
	minSuccessExpected int
	errorsCount        int
	successCount       int
}

func (c *SecuencialImp) registerSuccess() {
	c.successCount++
	c.errorsCount = 0

	if !c.healthy && c.successCount >= c.minSuccessExpected {
		now := time.Now()
		c.healthy = true
		c.healthySince = &now
		c.lastMessage = ""
	}
}

func (c *SecuencialImp) registerError(message string) {
	c.errorsCount++
	c.successCount = 0
	c.lastMessage = message

	if c.healthy && c.errorsCount >= c.maxErrorsAllowed {
		c.healthy = false
		c.healthySince = nil
	}
}

// NotifyServiceHit process hit
func (c *SecuencialImp) NotifyServiceHit(statusCode int, message string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if statusCode == 200 {
		c.logger.Debug(fmt.Sprintf("Hit success to %s, status code: %d", c.name, statusCode))
		c.registerSuccess()
	} else {
		c.logger.Debug(fmt.Sprintf("Hit error to %s, with status code: %d. Message: %s", c.name, statusCode, message))
		c.registerError(message)
	}

	now := time.Now()
	c.lastHit = &now
}

// NewCounterSecuencial create sucuencial counter
func NewCounterSecuencial(
	config Config,
	logger logging.LoggerInterface,
) *SecuencialImp {
	counter := &SecuencialImp{
		BaseCounterImp:     *NewBaseCounterImp(config.Name, config.Severity, logger),
		maxErrorsAllowed:   config.MaxErrorsAllowed,
		minSuccessExpected: config.MinSuccessExpected,
	}

	counter.task = asynctask.NewAsyncTask(config.Name, func(l logging.LoggerInterface) error {
		return config.TaskFunc(l, counter)
	}, config.TaskPeriod, nil, nil, logger)

	return counter
}
