package counter

import (
	"container/list"
	"fmt"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

// ByPercentageImp description
type ByPercentageImp struct {
	BaseCounterImp
	maxLen                int
	percentageToBeHealthy int
	cache                 *list.List
}

func (c *ByPercentageImp) calculateHealthy() {
	if c.cache.Len() == 0 {
		c.healthy = true
		return
	}

	okstatus := 0
	for e := c.cache.Front(); e != nil; e = e.Next() {
		if e.Value == 200 {
			okstatus++
		}
	}

	percentageok := okstatus * 100 / c.cache.Len()
	isHealthy := percentageok >= c.percentageToBeHealthy

	c.logger.Debug(fmt.Sprintf("%s alive: %v. Success percentage: %d", c.name, isHealthy, percentageok))

	if isHealthy && !c.healthy {
		now := time.Now().Unix()
		c.healthySince = &now
		c.lastMessage = ""
	} else if !isHealthy {
		c.healthySince = nil
	}

	c.healthy = isHealthy
}

// NotifyServiceHit process hit
func (c *ByPercentageImp) NotifyServiceHit(statusCode int, message string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.cache.Len() == c.maxLen {
		c.cache.Remove(c.cache.Front())
	}

	c.cache.PushBack(statusCode)

	if statusCode == 200 {
		c.logger.Debug(fmt.Sprintf("Hit success to %s, status code: %d", c.name, statusCode))
	} else {
		c.lastMessage = message
		c.logger.Debug(fmt.Sprintf("Hit error to %s, with status code: %d. Message: %s", c.name, statusCode, message))
	}

	c.calculateHealthy()

	now := time.Now().Unix()
	c.lastHit = &now
}

// NewCounterByPercentage new ByPercentage counter
func NewCounterByPercentage(
	config hcCommon.Config,
	logger logging.LoggerInterface,
) *ByPercentageImp {
	counter := &ByPercentageImp{
		BaseCounterImp:        *NewBaseCounterImp(config.Name, config.Severity, logger),
		maxLen:                config.MaxLen,
		cache:                 new(list.List),
		percentageToBeHealthy: config.PercentageToBeHealthy,
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
