package application

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application/counter"
)

// MonitorImp description
type MonitorImp struct {
	counters     []counter.ApplicationCounterInterface
	healthySince *time.Time
	lock         sync.RWMutex
	logger       logging.LoggerInterface
}

// HealthDto struct
type HealthDto struct {
	Healthy      bool       `json:"healthy"`
	HealthySince *time.Time `json:"healthySince"`
	Items        []ItemDto  `json:"items"`
}

// ItemDto struct
type ItemDto struct {
	Name       string     `json:"name"`
	Healthy    bool       `json:"healthy"`
	LastHit    *time.Time `json:"lastHit,omitempty"`
	ErrorCount int        `json:"errorCount,omitempty"`
	Severity   int        `json:"-"`
}

func (m *MonitorImp) getHealthySince(healthy bool) *time.Time {
	if !healthy {
		m.healthySince = nil
	}

	return m.healthySince
}

func checkIfIsHealthy(result []ItemDto) bool {
	for _, r := range result {
		if r.Healthy == false && r.Severity == counter.Critical {
			return false
		}
	}

	return true
}

// GetHealthStatus get application health
func (m *MonitorImp) GetHealthStatus() HealthDto {
	m.lock.Lock()
	defer m.lock.Unlock()

	var items []ItemDto

	for _, counter := range m.counters {
		res := counter.IsHealthy()
		items = append(items, ItemDto{
			Name:       res.Name,
			Healthy:    res.Healthy,
			LastHit:    res.LastHit,
			ErrorCount: res.ErrorCount,
			Severity:   res.Severity,
		})
	}

	healthy := checkIfIsHealthy(items)
	since := m.getHealthySince(healthy)

	return HealthDto{
		Healthy:      healthy,
		Items:        items,
		HealthySince: since,
	}
}

// NotifyEvent notify to counter an event
func (m *MonitorImp) NotifyEvent(monitorType int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		if counter.GetMonitorType() == monitorType {
			counter.NotifyEvent()
		}
	}
}

// Reset counter value
func (m *MonitorImp) Reset(monitorType int, value int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		if counter.GetMonitorType() == monitorType {
			counter.Reset(value)
		}
	}
}

// Start counters
func (m *MonitorImp) Start() {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		counter.Start()
	}
}

// Stop counters
func (m *MonitorImp) Stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		counter.Stop()
	}
}

// NewMonitorImp create a new application monitor
func NewMonitorImp(
	cfgs []*counter.Config,
	logger logging.LoggerInterface,
) *MonitorImp {
	var appcounters []counter.ApplicationCounterInterface

	for _, cfg := range cfgs {
		switch cfg.CounterType {
		case counter.Threshold:
			appcounters = append(appcounters, counter.NewThresholdCounter(cfg, logger))
		default:
			appcounters = append(appcounters, counter.NewPeriodicCounter(cfg, logger))
		}
	}

	now := time.Now()
	return &MonitorImp{
		logger:       logger,
		counters:     appcounters,
		healthySince: &now,
	}
}
