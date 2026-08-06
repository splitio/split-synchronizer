package evcalc

import (
	"sync"
	"time"
)

// Monitor specifies the interface for a lambda calculator
type Monitor interface {
	StoreDataFlushed(timestamp time.Time, countFlushed int, countInStorage int64)
	Lambda() float64
	Acquire() bool
	Release()
	Busy() bool
}

// record struct that has all the required information of one flushing process
type record struct {
	Timestamp     time.Time
	DataFlushed   int
	DataInStorage int64
}

// MonitorImpl struct that will has a window of statistics for eviction lambda calculation
type MonitorImpl struct {
	flushingStats []record
	maxLength     int
	mutex         sync.RWMutex
	lambda        float64
	busy          bool
}

// New constructs a new eviction calculation monitor
func New(threads int) *MonitorImpl {
	return &MonitorImpl{
		flushingStats: make([]record, 0),
		maxLength:     100 * threads,
		lambda:        1,
	}
}

// StoreDataFlushed stores data flushed into the monitor
func (m *MonitorImpl) StoreDataFlushed(timestamp time.Time, countFlushed int, countInStorage int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.flushingStats) >= m.maxLength {
		m.flushingStats = m.flushingStats[1:m.maxLength]
	}
	m.flushingStats = append(m.flushingStats, record{
		Timestamp:     timestamp,
		DataFlushed:   countFlushed,
		DataInStorage: countInStorage,
	})
	m.lambda = m.calculateLambda()
}

// Lambda the returns the last known lambda value
func (m *MonitorImpl) Lambda() float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.lambda
}

// Acquire requests permission to flush whichever resource this monitor is associated to
func (m *MonitorImpl) Acquire() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.busy {
		return false
	}

	m.busy = true
	return true
}

// Release signals the end of a syncrhonization operation which was previously acquired
func (m *MonitorImpl) Release() {
	m.mutex.Lock()
	m.busy = false
	m.mutex.Unlock()
}

// Busy returns true if the permission is currently acquired and hasn't yet been released
func (m *MonitorImpl) Busy() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.busy
}

func calculateAmountFlushed(records []record) int {
	amountFlushed := 0
	for _, i := range records {
		amountFlushed += i.DataFlushed
	}
	return amountFlushed
}

func (m *MonitorImpl) calculateLambda() float64 {
	t := int64(calculateAmountFlushed(m.flushingStats))
	dataInT1 := m.flushingStats[0].DataInStorage
	dataInT2 := m.flushingStats[len(m.flushingStats)-1].DataInStorage
	amountGeneratedBetweenT1andT2 := float64(dataInT2 - dataInT1 + t)

	if amountGeneratedBetweenT1andT2 == 0 {
		return 1
	}
	return float64(t) / amountGeneratedBetweenT1andT2
}

var _ Monitor = (*MonitorImpl)(nil)
