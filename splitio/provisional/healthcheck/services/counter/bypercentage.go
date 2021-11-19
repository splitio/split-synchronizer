package counter

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"

	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	// Critical severity
	Critical = iota
	// Degraded severity
	Degraded
	// Low severity
	Low
)

// ByPercentageImp description
type ByPercentageImp struct {
	lock                  sync.RWMutex
	logger                logging.LoggerInterface
	severity              int
	lastMessage           string
	lastHit               *time.Time
	healthy               bool
	healthySince          *time.Time
	name                  string
	task                  *asynctask.AsyncTask
	url                   string
	maxLen                int
	percentageToBeHealthy int
	cache                 *list.List
}

// ServicesCounterInterface interface
type ServicesCounterInterface interface {
	NotifyHit(statusCode int, message string)
	IsHealthy() HealthyResult
	Start()
	Stop()
}

// HealthyResult result
type HealthyResult struct {
	URL          string
	Severity     int
	Healthy      bool
	LastMessage  string
	HealthySince *time.Time
	LastHit      *time.Time
}

// Config counter config
type Config struct {
	MaxErrorsAllowed      int
	MinSuccessExpected    int
	MaxLen                int
	PercentageToBeHealthy int
	Name                  string
	ServiceURL            string
	ServiceHealthEndpoint string
	Severity              int
	TaskPeriod            int
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

// NotifyHit process hit
func (c *ByPercentageImp) NotifyHit(statusCode int, message string) {
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

// IsHealthy return counter health
func (c *ByPercentageImp) IsHealthy() HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return HealthyResult{
		URL:          c.url,
		Severity:     c.severity,
		Healthy:      c.healthy,
		LastMessage:  c.lastMessage,
		HealthySince: c.healthySince,
		LastHit:      c.lastHit,
	}
}

// Start counter task
func (c *ByPercentageImp) Start() {
	c.task.Start()
}

// Stop counter task
func (c *ByPercentageImp) Stop() {
	c.task.Stop(false)
}

// NewCounterByPercentage new ByPercentage counter
func NewCounterByPercentage(
	config Config,
	logger logging.LoggerInterface,
) *ByPercentageImp {
	now := time.Now()
	counter := &ByPercentageImp{
		name:                  config.Name,
		lock:                  sync.RWMutex{},
		logger:                logger,
		severity:              config.Severity,
		healthy:               true,
		healthySince:          &now,
		url:                   config.ServiceURL + config.ServiceHealthEndpoint,
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

		counter.NotifyHit(status, message)

		return nil
	}

	counter.task = asynctask.NewAsyncTask(config.Name, taskFunc, config.TaskPeriod, nil, nil, logger)

	return counter
}

// DefaultConfig new config with default values
func DefaultConfig(
	name string,
	url string,
	endpoint string,
) Config {
	return Config{
		MaxLen:                10,
		PercentageToBeHealthy: 70,
		Name:                  name,
		ServiceURL:            url,
		TaskPeriod:            3600,
		ServiceHealthEndpoint: endpoint,
		Severity:              Critical,
	}
}
