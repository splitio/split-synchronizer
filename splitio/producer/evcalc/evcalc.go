package evcalc

import "sync"

// Monitor specifies the interface for a lambda calculator
type Monitor interface {
	StoreDataFlushed(timestamp int64, countFlushed int, countInStorage int64)
	Lambda() float64
}

// record struct that has all the required information of one flushing process
type record struct {
	Timestamp     int64
	DataFlushed   int
	DataInStorage int64
}

// MonitorImpl struct that will has a window of statistics for eviction lambda calculation
type MonitorImpl struct {
	flushingStats []record
	maxLength     int
	mutex         sync.RWMutex
	lambda        float64
}

// New constructs a new eviction calculation monitor
func New(threads int) *MonitorImpl {
	return &MonitorImpl{
		flushingStats: make([]record, 0),
		maxLength:     100 * threads,
		lambda:        1,
	}
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
	return float64(t) / amountGeneratedBetweenT1andT2
}

// StoreDataFlushed stores data flushed into the monitor
func (m *MonitorImpl) StoreDataFlushed(timestamp int64, countFlushed int, countInStorage int64) {
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
