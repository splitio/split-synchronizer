package counter

import (
	"sync"
	"time"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
)

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
func (c *baseCounterImp) IsHealthy() hcCommon.HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

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
func (c *baseCounterImp) Start() {
	c.task.Start()
}

// Stop counter task
func (c *baseCounterImp) Stop() {
	c.task.Stop(false)
}
