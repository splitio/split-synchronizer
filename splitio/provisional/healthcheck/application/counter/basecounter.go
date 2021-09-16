package counter

import (
	"sync"
	"time"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/logging"
)

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

// UpdateLastHit update last hit
func (c *applicationCounterImp) UpdateLastHit() {
	now := time.Now()
	c.lastHit = &now
}

// GetMonitorType return monitor type
func (c *applicationCounterImp) GetMonitorType() int {
	return c.monitorType
}

// IsHealthy return the counter health
func (c *applicationCounterImp) IsHealthy() hcCommon.HealthyResult {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return hcCommon.HealthyResult{
		Name:       c.name,
		Healthy:    c.healthy,
		Severity:   c.severity,
		LastHit:    c.lastHit,
		ErrorCount: c.errorCount,
	}
}
