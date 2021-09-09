package counter

import (
	"sync"
	"time"

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
	TaskFunc              func(l logging.LoggerInterface, c BaseCounterInterface) error
	TaskPeriod            int
}

// BaseCounterInterface interface
type BaseCounterInterface interface {
	NotifyServiceHit(statusCode int, message string)
	IsHealthy() HealthyResult
	Start()
	Stop()
}

// BaseCounterImp counter implementatiom
type BaseCounterImp struct {
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

// HealthyResult result
type HealthyResult struct {
	Name         string
	Severity     int
	Healthy      bool
	LastMessage  string
	HealthySince *time.Time
	LastHit      *time.Time
}

// IsHealthy return counter health
func (c *BaseCounterImp) IsHealthy() HealthyResult {
	c.lock.Lock()
	defer c.lock.Unlock()

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
func (c *BaseCounterImp) Start() {
	c.task.Start()
}

// Stop counter task
func (c *BaseCounterImp) Stop() {
	c.task.Stop(false)
}

// NewBaseCounterImp description
func NewBaseCounterImp(
	name string,
	severity int,
	logger logging.LoggerInterface,
) *BaseCounterImp {
	now := time.Now()
	return &BaseCounterImp{
		name:         name,
		lock:         sync.RWMutex{},
		logger:       logger,
		severity:     severity,
		healthy:      true,
		healthySince: &now,
	}
}
