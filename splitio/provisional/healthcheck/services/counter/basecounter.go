package counter

import (
	"sync"
	"time"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

// BaseCounterImp counter implementatiom
type BaseCounterImp struct {
	lock         sync.RWMutex
	logger       logging.LoggerInterface
	severity     int
	lastMessage  string
	lastHit      *int64
	healthy      bool
	healthySince *int64
	name         string
	task         *asynctask.AsyncTask
}

// IsHealthy return counter health
func (c *BaseCounterImp) IsHealthy() hcCommon.HealthyResult {
	c.lock.Lock()
	defer c.lock.Unlock()

	return hcCommon.HealthyResult{
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
	now := time.Now().Unix()
	return &BaseCounterImp{
		name:         name,
		lock:         sync.RWMutex{},
		logger:       logger,
		severity:     severity,
		healthy:      true,
		healthySince: &now,
	}
}
