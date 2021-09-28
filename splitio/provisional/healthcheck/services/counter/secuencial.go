package counter

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"

	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

// SecuencialImp description
type SecuencialImp struct {
	baseCounterImp
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
	config *Config,
	logger logging.LoggerInterface,
) *SecuencialImp {
	now := time.Now()
	counter := &SecuencialImp{
		baseCounterImp: baseCounterImp{
			name:         config.Name,
			lock:         sync.RWMutex{},
			logger:       logger,
			severity:     config.Severity,
			healthy:      true,
			healthySince: &now,
		},
		maxErrorsAllowed:   config.MaxErrorsAllowed,
		minSuccessExpected: config.MinSuccessExpected,
	}

	client := api.NewHTTPClient("", conf.AdvancedConfig{}, config.ServiceURL, logger, dtos.Metadata{})

	taskFunc := func(logger logging.LoggerInterface) error {
		status := 200
		message := ""

		_, err := client.Get(config.ServiceHealthEndpoint, nil)
		if err != nil {
			status = -1
			message = err.Error()
			if httperror, ok := err.(*dtos.HTTPError); ok {
				status = httperror.Code
				message = httperror.Message
			}

		}

		counter.NotifyServiceHit(status, message)

		return nil
	}

	counter.task = asynctask.NewAsyncTask(config.Name, taskFunc, config.TaskPeriod, nil, nil, logger)

	return counter
}
