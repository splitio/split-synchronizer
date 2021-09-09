package counter

import (
	"container/list"
	"fmt"
	"time"

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
		now := time.Now()
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

	now := time.Now()
	c.lastHit = &now
}

// NewCounterByPercentage new ByPercentage counter
func NewCounterByPercentage(
	config Config,
	logger logging.LoggerInterface,
) *ByPercentageImp {
	counter := &ByPercentageImp{
		BaseCounterImp:        *NewBaseCounterImp(config.Name, config.Severity, logger),
		maxLen:                config.MaxLen,
		cache:                 new(list.List),
		percentageToBeHealthy: config.PercentageToBeHealthy,
	}

	counter.task = asynctask.NewAsyncTask(config.Name, func(l logging.LoggerInterface) error {
		return config.TaskFunc(l, counter)
	}, config.TaskPeriod, nil, nil, logger)

	return counter
}
