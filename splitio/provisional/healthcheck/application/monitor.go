package application

import (
	"fmt"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	toolkitsync "github.com/splitio/go-toolkit/v5/sync"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application/counter"
)

// MonitorIterface monitor interface
type MonitorIterface interface {
	GetHealthStatus() HealthDto
	NotifyEvent(counterType int)
	Reset(counterType int, value int)
	Start()
	Stop()
}

// MonitorImp description
type MonitorImp struct {
	splitsCounter   counter.ThresholdCounterInterface
	segmentsCounter counter.ThresholdCounterInterface
	storageCounter  counter.PeriodicCounterInterface
	producerMode    toolkitsync.AtomicBool
	healthySince    *time.Time
	lock            sync.RWMutex
	logger          logging.LoggerInterface
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
	m.lock.RLock()
	defer m.lock.RUnlock()

	var items []ItemDto
	var results []counter.HealthyResult
	results = append(results, m.splitsCounter.IsHealthy(), m.segmentsCounter.IsHealthy())

	if m.producerMode.IsSet() {
		results = append(results, m.storageCounter.IsHealthy())
	}

	for _, res := range results {
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
func (m *MonitorImp) NotifyEvent(counterType int) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	m.logger.Debug(fmt.Sprintf("Notify Event. Type: %d.", counterType))

	switch counterType {
	case counter.Splits:
		m.splitsCounter.NotifyHit()
	case counter.Segments:
		m.segmentsCounter.NotifyHit()
	}
}

// Reset counter value
func (m *MonitorImp) Reset(counterType int, value int) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	m.logger.Debug(fmt.Sprintf("Reset. Type: %d. Value: %d", counterType, value))

	switch counterType {
	case counter.Splits:
		m.splitsCounter.ResetThreshold(value)
	case counter.Segments:
		m.segmentsCounter.ResetThreshold(value)
	}
}

// Start counters
func (m *MonitorImp) Start() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.splitsCounter.Start()
	m.segmentsCounter.Start()
	if m.producerMode.IsSet() {
		m.storageCounter.Start()
	}

	m.logger.Debug("Application Monitor started.")
}

// Stop counters
func (m *MonitorImp) Stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.splitsCounter.Stop()
	m.segmentsCounter.Stop()

	if m.producerMode.IsSet() {
		m.storageCounter.Stop()
	}
}

// NewMonitorImp create a new application monitor
func NewMonitorImp(
	splitsConfig counter.ThresholdConfig,
	segmentsConfig counter.ThresholdConfig,
	storageConfig *counter.PeriodicConfig,
	logger logging.LoggerInterface,
) *MonitorImp {
	now := time.Now()
	monitor := &MonitorImp{
		splitsCounter:   counter.NewThresholdCounter(splitsConfig, logger),
		segmentsCounter: counter.NewThresholdCounter(segmentsConfig, logger),
		producerMode:    *toolkitsync.NewAtomicBool(storageConfig != nil),
		logger:          logger,
		healthySince:    &now,
	}

	if monitor.producerMode.IsSet() {
		monitor.storageCounter = counter.NewPeriodicCounter(*storageConfig, logger)
	}

	return monitor
}
