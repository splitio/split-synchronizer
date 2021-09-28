package counter

import (
	"sync"
	"time"

	//hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	// ByPercentage counter type
	ByPercentage = iota
	// Sequential counter type
	Sequential
)

const (
	// Critical severity
	Critical = iota
	// Degraded severity
	Degraded
	// Low severity
	Low
)

// ServicesCounterInterface interface
type ServicesCounterInterface interface {
	NotifyServiceHit(statusCode int, message string)
	IsHealthy() HealthyResult
	Start()
	Stop()
}

// HealthyResult result
type HealthyResult struct {
	Name         string
	Severity     int
	Healthy      bool
	LastMessage  string
	HealthySince *time.Time
	LastHit      *time.Time
}

// Config counter config
type Config struct {
	CounterType           int
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

type baseCounterImp struct {
	lock         sync.RWMutex
	logger       logging.LoggerInterface
	severity     int
	lastMessage  string
	lastHit      *time.Time
	healthy      bool
	healthySince *time.Time
	name         string
	task         *asynctask.AsyncTask
}

// IsHealthy return counter health
func (c *baseCounterImp) IsHealthy() HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return HealthyResult{
		Name:         c.name,
		Severity:     c.severity,
		Healthy:      c.healthy,
		LastMessage:  c.lastMessage,
		HealthySince: c.healthySince,
		LastHit:      c.lastHit,
	}
}

// Start counter task
func (c *baseCounterImp) Start() {
	c.task.Start()
}

// Stop counter task
func (c *baseCounterImp) Stop() {
	c.task.Stop(false)
}

// NewServicesConfig new config with default values
func NewServicesConfig(
	name string,
	url string,
	endpoint string,
) *Config {
	return &Config{
		CounterType:           ByPercentage,
		MaxLen:                10,
		PercentageToBeHealthy: 70,
		Name:                  name,
		ServiceURL:            url,
		TaskPeriod:            3600,
		ServiceHealthEndpoint: endpoint,
		Severity:              Critical,
	}
}
