package services

import (
	"sync"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services/counter"
)

const (
	healthyStatus  = "healthy"
	downStatus     = "down"
	degradedStatus = "degraded"
)

// MonitorImp description
type MonitorImp struct {
	Counters []counter.BaseCounterInterface
	lock     sync.RWMutex
	logger   logging.LoggerInterface
}

// Start stop counters
func (m *MonitorImp) Start() {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, c := range m.Counters {
		c.Start()
	}
}

// Stop stop counters
func (m *MonitorImp) Stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, c := range m.Counters {
		c.Stop()
	}
}

// GetHealthStatus return services health
func (m *MonitorImp) GetHealthStatus() hcCommon.HealthDto {
	m.lock.Lock()
	defer m.lock.Unlock()

	var items []hcCommon.ItemDto

	criticalCount := 0
	degradedCount := 0

	for _, c := range m.Counters {
		res := c.IsHealthy()

		if !res.Healthy {
			switch res.Severity {
			case counter.Critical:
				criticalCount++
			case counter.Degraded:
				degradedCount++
			}
		}

		items = append(items, hcCommon.ItemDto{
			Service:      res.Name,
			Healthy:      res.Healthy,
			Message:      res.LastMessage,
			HealthySince: res.HealthySince,
			LastHit:      res.LastHit,
		})
	}

	status := healthyStatus

	if criticalCount > 0 {
		status = downStatus
	} else if degradedCount > 0 {
		status = degradedStatus
	}

	return hcCommon.HealthDto{
		Status: status,
		Items:  items,
	}
}

// NewMonitorImp create services monitor
func NewMonitorImp(
	cfgs []counter.Config,
	logger logging.LoggerInterface,
) *MonitorImp {
	var serviceCounters []counter.BaseCounterInterface

	for _, cfg := range cfgs {
		switch cfg.CounterType {
		case hcCommon.ByPercentage:
			serviceCounters = append(serviceCounters, counter.NewCounterByPercentage(cfg, logger))
		default:
			serviceCounters = append(serviceCounters, counter.NewCounterSecuencial(cfg, logger))
		}
	}

	return &MonitorImp{
		Counters: serviceCounters,
	}
}
