package counter

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	// Periodic counter type
	Periodic = iota
	// Threshold counter type
	Threshold
)

const (
	// Critical severity
	Critical = iota
	// Low severity
	Low
)

// ApplicationCounterInterface application counter interface
type ApplicationCounterInterface interface {
	IsHealthy() HealthyResult
	NotifyEvent()
	Reset(value int) error
	GetMonitorType() int
	UpdateLastHit()
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

// Config counter configuration
type Config struct {
	Name                     string
	CounterType              int
	MonitorType              int
	TaskFunc                 func(l logging.LoggerInterface, c ApplicationCounterInterface) error
	GoroutineFunc            func(c ApplicationCounterInterface)
	Period                   int
	MaxErrorsAllowedInPeriod int
	Severity                 int
}

type applicationCounterImp struct {
	name        string
	counterType int
	monitorType int
	lastHit     *time.Time
	healthy     bool
	running     bool
	period      int
	severity    int
	errorCount  int
	lock        sync.RWMutex
	logger      logging.LoggerInterface
}

func (c *applicationCounterImp) updateLastHit() {
	now := time.Now()
	c.lastHit = &now
}

// UpdateLastHit update last hit
func (c *applicationCounterImp) UpdateLastHit() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.updateLastHit()
}

// GetMonitorType return monitor type
func (c *applicationCounterImp) GetMonitorType() int {
	return c.monitorType
}

// IsHealthy return the counter health
func (c *applicationCounterImp) IsHealthy() HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return HealthyResult{
		Name:       c.name,
		Healthy:    c.healthy,
		Severity:   c.severity,
		LastHit:    c.lastHit,
		ErrorCount: c.errorCount,
	}
}

// NewApplicationConfig new config with default values
func NewApplicationConfig(
	name string,
	monitorType int,
) *Config {
	return &Config{
		Name:        name,
		MonitorType: monitorType,
		CounterType: Threshold,
		Period:      3600,
		Severity:    Critical,
	}
}
