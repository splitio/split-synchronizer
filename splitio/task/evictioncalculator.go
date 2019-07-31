package task

import (
	"sync"

	"github.com/splitio/split-synchronizer/conf"
)

// record struct that has all the required information of one flushing process
type record struct {
	Timestamp     int64
	DataFlushed   int
	DataInStorage int64
}

// monitor struct that will has a window of statistics for eviction delta calculation
type monitor struct {
	FlushingStats []record
	MaxLength     int
	Mutex         sync.RWMutex
	Delta         float64
}

var eventsMonitor monitor
var impressionsMonitor monitor

// InitializeEvictionCalculator initializes the eviction calculator module
func InitializeEvictionCalculator() {
	eventsMonitor = monitor{
		FlushingStats: make([]record, 0),
		MaxLength:     int(100) * conf.Data.EventsThreads,
		Delta:         1,
	}
	impressionsMonitor = monitor{
		FlushingStats: make([]record, 0),
		MaxLength:     int(100) * conf.Data.ImpressionsThreads,
		Delta:         1,
	}
}

// storeRecord stores a record depending on the length. It will add one more element if the array is not full or shift the array one place
func storeRecord(stats record, records *[]record, maxLength int) {
	if len(*records) >= maxLength {
		*records = (*records)[1:maxLength]
	}
	*records = append(*records, stats)
}

func calculateAmountFlushed(records []record) int {
	amountFlushed := 0
	for _, i := range records {
		amountFlushed += i.DataFlushed
	}
	return amountFlushed
}

func calculateDelta(records []record) float64 {
	t := int64(calculateAmountFlushed(records))

	// grabs the quantity of elements for the first record
	dataInT1 := records[0].DataInStorage
	// grabs the quantity of elements for the last record
	dataInT2 := records[len(records)-1].DataInStorage
	// calculates the total amount of elements generated between T1 and T2
	amountGeneratedBetweenT1andT2 := float64(dataInT2 - dataInT1 + t)

	return float64(t) / amountGeneratedBetweenT1andT2
}

// StoreDataFlushed stores data flushed into the monitor
func StoreDataFlushed(timestamp int64, countFlushed int, countInStorage int64, operation string) {
	var newInformation = record{
		Timestamp:     timestamp,
		DataFlushed:   countFlushed,
		DataInStorage: countInStorage,
	}
	if operation == "events" {
		eventsMonitor.Mutex.Lock()
		storeRecord(newInformation, &eventsMonitor.FlushingStats, eventsMonitor.MaxLength)
		eventsMonitor.Delta = calculateDelta(eventsMonitor.FlushingStats)
		eventsMonitor.Mutex.Unlock()
	} else {
		impressionsMonitor.Mutex.Lock()
		storeRecord(newInformation, &impressionsMonitor.FlushingStats, impressionsMonitor.MaxLength)
		impressionsMonitor.Delta = calculateDelta(impressionsMonitor.FlushingStats)
		impressionsMonitor.Mutex.Unlock()
	}
}

// GetImpressionsDelta returns eviction factor for impressions
func GetImpressionsDelta() float64 {
	return impressionsMonitor.Delta
}

// GetEventsDelta returns eviction factor for events
func GetEventsDelta() float64 {
	return eventsMonitor.Delta
}
