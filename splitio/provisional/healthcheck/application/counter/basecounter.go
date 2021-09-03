package counter

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/logging"
)

// BaseCounterInterface description
type BaseCounterInterface interface {
	IsHealthy() bool
	NotifyEvent()
	Reset(value int) error
	GetType() int
	GetLastHit() *time.Time
	GetName() string
	Start()
	Stop()
	GetErrorsCount() *int
	GetSeverity() int
}

const (
	// Splits counter type
	Splits = iota
	// Segments counter type
	Segments
	// Storage counter type
	Storage
	// SyncErros counter type
	SyncErros
)

const (
	// Critical severity
	Critical = iota
	// Low severity
	Low
)

// Config description
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
	lock        sync.RWMutex
	logger      logging.LoggerInterface
}

func (c *ApplicationCounterImp) updateLastHit() {
	now := time.Now()
	c.lastHit = &now
}

// GetSeverity description
func (c *ApplicationCounterImp) GetSeverity() int {
	return c.severity
}

// GetType description
func (c *ApplicationCounterImp) GetType() int {
	return c.counterType
}

// GetLastHit description
func (c *ApplicationCounterImp) GetLastHit() *time.Time {
	return c.lastHit
}

// GetName description
func (c *ApplicationCounterImp) GetName() string {
	return c.name
}

// IsHealthy description
func (c *ApplicationCounterImp) IsHealthy() bool {
	return c.healthy
}

// NewApplicationCounterImp description
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
