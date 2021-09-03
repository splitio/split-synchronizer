package services

import (
	"sync"
	"time"

	"github.com/splitio/go-toolkit/logging"
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

// HealthDto description
type HealthDto struct {
	Status       string         `json:"serviceStatus"`
	Dependencies []DependecyDto `json:"dependencies"`
}

// DependecyDto description
type DependecyDto struct {
	Service      string     `json:"service"`
	Healthy      bool       `json:"healthy"`
	Message      string     `json:"message,omitempty"`
	HealthySince *time.Time `json:"healthySince,omitempty"`
	LastHit      *time.Time `json:"lastHit,omitempty"`
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

// GetHealthStatus description
func (m *MonitorImp) GetHealthStatus() HealthDto {
	m.lock.Lock()
	defer m.lock.Unlock()

	var dependencies []DependecyDto

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

		dependencies = append(dependencies, DependecyDto{
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

	return HealthDto{
		Status:       status,
		Dependencies: dependencies,
	}
}

// NewMonitorImp description
func NewMonitorImp(
	cfgs []counter.Config,
	logger logging.LoggerInterface,
) *MonitorImp {
	var serviceCounters []counter.BaseCounterInterface

	for _, cfg := range cfgs {
		switch cfg.CounterType {
		case counter.ByPercentage:
			serviceCounters = append(serviceCounters, counter.NewCounterByPercentage(cfg, logger))
		default:
			serviceCounters = append(serviceCounters, counter.NewCounterSecuencial(cfg, logger))
		}
	}

	return &MonitorImp{
		Counters: serviceCounters,
	}
}
