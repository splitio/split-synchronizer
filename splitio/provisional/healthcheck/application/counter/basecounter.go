package counter

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

// BaseCounterInterface application counter interface
type BaseCounterInterface interface {
	IsHealthy() HealthyResult
	NotifyEvent()
	Reset(value int) error
	GetType() int
	Start()
	Stop()
}

// HealthyResult description
type HealthyResult struct {
	Name       string
	Severity   int
	Healthy    bool
	LastHit    *time.Time
	ErrorCount int
}

const (
	// Critical severity
	Critical = iota
	// Low severity
	Low
)

// Config counter configuration
type Config struct {
	Name                     string
	CounterType              int
	Periodic                 bool
	TaskFunc                 func(l logging.LoggerInterface, c BaseCounterInterface) error
	Period                   int
	MaxErrorsAllowedInPeriod int
	Severity                 int
}

// ApplicationCounterImp description
type ApplicationCounterImp struct {
	name        string
	counterType int
	lastHit     *time.Time
	healthy     bool
	running     bool
	period      int
	severity    int
	errorCount  int
	lock        sync.RWMutex
	logger      logging.LoggerInterface
}

func (c *ApplicationCounterImp) updateLastHit() {
	now := time.Now()
	c.lastHit = &now
}

// GetType return counter type
func (c *ApplicationCounterImp) GetType() int {
	return c.counterType
}

// IsHealthy return the counter health
func (c *ApplicationCounterImp) IsHealthy() HealthyResult {
	return HealthyResult{
		Name:       c.name,
		Healthy:    c.healthy,
		Severity:   c.severity,
		LastHit:    c.lastHit,
		ErrorCount: c.errorCount,
	}
}

// NewApplicationCounterImp create an application counter
func NewApplicationCounterImp(
	name string,
	counterType int,
	period int,
	severity int,
	logger logging.LoggerInterface,
) *ApplicationCounterImp {
	return &ApplicationCounterImp{
		name:        name,
		lock:        sync.RWMutex{},
		logger:      logger,
		healthy:     true,
		running:     false,
		counterType: counterType,
		period:      period,
		severity:    severity,
	}
}
