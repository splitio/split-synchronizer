package application

import (
	"sync"
	"time"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application/counter"
)

// MonitorImp description
type MonitorImp struct {
	counters     []hcCommon.CounterInterface
	healthySince *int64
	lock         sync.RWMutex
	logger       logging.LoggerInterface
}

func (m *MonitorImp) getHealthySince(healthy bool) *int64 {
	if !healthy {
		m.healthySince = nil
	}

	return m.healthySince
}

func checkIfIsHealthy(result []hcCommon.ItemDto) bool {
	for _, r := range result {
		if r.Healthy == false && r.Severity == hcCommon.Critical {
			return false
		}
	}

	return true
}

// GetHealthStatus get application health
func (m *MonitorImp) GetHealthStatus() hcCommon.HealthDto {
	m.lock.Lock()
	defer m.lock.Unlock()

	var items []hcCommon.ItemDto

	for _, counter := range m.counters {
		res := counter.IsHealthy()
		items = append(items, hcCommon.ItemDto{
			Name:       res.Name,
			Healthy:    res.Healthy,
			LastHit:    res.LastHit,
			ErrorCount: res.ErrorCount,
			Severity:   res.Severity,
		})
	}

	healthy := checkIfIsHealthy(items)
	since := m.getHealthySince(healthy)

	return hcCommon.HealthDto{
		Healthy:      healthy,
		Items:        items,
		HealthySince: since,
	}
}

// NotifyEvent notify to counter an event
func (m *MonitorImp) NotifyEvent(counterType int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		if counter.GetType() == counterType {
			counter.NotifyEvent()
		}
	}
}

// Reset counter value
func (m *MonitorImp) Reset(counterType int, value int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, counter := range m.counters {
		if counter.GetType() == counterType {
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
	cfgs []*hcCommon.Config,
	logger logging.LoggerInterface,
) *MonitorImp {
	var appcounters []hcCommon.CounterInterface

	for _, cfg := range cfgs {
		if cfg.Periodic {
			appcounters = append(appcounters, counter.NewCounterPeriodic(cfg, logger))
		} else {
			appcounters = append(appcounters, counter.NewCounterThresholdImp(cfg, logger))
		}
	}

	now := time.Now().Unix()
	return &MonitorImp{
		logger:       logger,
		counters:     appcounters,
		healthySince: &now,
	}
}
