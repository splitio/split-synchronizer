package stats

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const lastStoredLatencies = 500

var notStorageInitialiazedError = errors.New("Stats storage has not been initialized")
var storageInitialized = false
var startTime time.Time
var countersStorage *CounterStorage
var latenciesStorage *LatencyStorage

func init() {
	startTime = time.Now()
}

//------------------------------------------------------------------------------
// COUNTERS STORAGE
//------------------------------------------------------------------------------

// CounterStorage struct to storage counters in memory
type CounterStorage struct {
	counters map[string]int64
	mutext   *sync.RWMutex
}

// Add adds a counter.
func (c *CounterStorage) Add(name string, value int64) {
	c.mutext.Lock()
	c.counters[name] += value
	c.mutext.Unlock()
}

// Counters returns counters
func (c *CounterStorage) Counters() map[string]int64 {
	var countersToReturn = make(map[string]int64)
	c.mutext.RLock()
	for k, v := range c.counters {
		countersToReturn[k] = v
	}
	c.mutext.RUnlock()
	return countersToReturn
}

//------------------------------------------------------------------------------
//LATENCIES STORAGE
//------------------------------------------------------------------------------

type LatencyStorageAddFunc func(string, []int64) error

// LatencyStorage struct to storage latencies in memory
type LatencyStorage struct {
	latencies map[string][]int64
	mutext    *sync.RWMutex
}

// Add adds a latency
func (l *LatencyStorage) Add(name string, value []int64) {
	l.mutext.Lock()

	if l.latencies[name] == nil {
		l.latencies[name] = make([]int64, 0)
	}

	l.latencies[name] = append(l.latencies[name], value...)

	if len(l.latencies[name]) > lastStoredLatencies {
		start := len(l.latencies[name]) - lastStoredLatencies
		l.latencies[name] = l.latencies[name][start:]
	}
	l.mutext.Unlock()
}

// AddBkt adds a latency bucket
func (l *LatencyStorage) AddBkt(name string, value []int64) {
	l.mutext.Lock()

	if l.latencies[name] == nil {
		l.latencies[name] = make([]int64, 23)
	}

	for i, v := range l.latencies[name] {
		l.latencies[name][i] = v + value[i]
	}

	l.mutext.Unlock()
}

// Latencies returns latencies
func (l *LatencyStorage) Latencies() map[string][]int64 {
	var toReturn = make(map[string][]int64)
	l.mutext.RLock()
	for k, v := range l.latencies {
		toReturn[k] = v
	}
	l.mutext.RUnlock()
	return toReturn
}

//------------------------------------------------------------------------------
// Stats API
//------------------------------------------------------------------------------

// Initialize stats
func Initialize() {
	countersStorage = &CounterStorage{counters: make(map[string]int64), mutext: &sync.RWMutex{}}
	latenciesStorage = &LatencyStorage{latencies: make(map[string][]int64), mutext: &sync.RWMutex{}}
	storageInitialized = true
}

// StorageInitialized returns true if storage has been Initialized
func StorageInitialized() bool {
	return storageInitialized
}

// Uptime returns a time.Duration since startTIme
func Uptime() time.Duration {
	return time.Since(startTime)
}

func UptimeFormated() string {
	upt := time.Since(startTime)
	d := int64(0)
	h := int64(0)
	m := int64(0)
	s := int64(upt.Seconds())

	if s > 60 {
		m = int64(s / 60)
		s = s - m*60
	}

	if m > 60 {
		h = int64(m / 60)
		m = m - h*60
	}

	if h > 24 {
		d = int64(h / 24)
		h = h - d*24
	}

	return fmt.Sprintf("%dd %dh %dm %ds", d, h, m, s)
}

// SaveCounter saves counter value
func SaveCounter(name string, value int64) error {
	if !storageInitialized {
		return notStorageInitialiazedError
	}
	countersStorage.Add(name, value)
	return nil
}

// Counters returns a counters map
func Counters() map[string]int64 {
	return countersStorage.Counters()
}

// SaveLatency saves the last N latencies for a given metric
func SaveLatency(name string, latencies []int64) error {
	if !storageInitialized {
		return notStorageInitialiazedError
	}
	latenciesStorage.Add(name, latencies)
	return nil
}

// SaveLatencyBkt saves the latencies for a given metric
func SaveLatencyBkt(name string, latencies []int64) error {
	if !storageInitialized {
		return notStorageInitialiazedError
	}
	latenciesStorage.AddBkt(name, latencies)
	return nil
}

// Latencies returns a latencies map
func Latencies() map[string][]int64 {
	return latenciesStorage.Latencies()
}
