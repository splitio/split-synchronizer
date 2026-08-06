package counter

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	toolkitsync "github.com/splitio/go-toolkit/v5/sync"
)

const (
	// Critical severity
	Critical = iota
	// Low severity
	Low
)

// HealthyResult description
type HealthyResult struct {
	Name       string
	Severity   int
	Healthy    bool
	LastHit    *time.Time
	ErrorCount int
}

type applicationCounterImp struct {
	name     string
	lastHit  *time.Time
	healthy  bool
	running  toolkitsync.AtomicBool
	period   int
	severity int
	lock     sync.RWMutex
	logger   logging.LoggerInterface
}

func (c *applicationCounterImp) updateLastHit() {
	now := time.Now()
	c.lastHit = &now
}
