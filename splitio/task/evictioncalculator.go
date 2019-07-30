package task

import (
	"sync"

	"github.com/splitio/split-synchronizer/conf"
)

type record struct {
	Timestamp     int64
	DataFlushed   int
	DataInStorage int64
}

var eventsFlushingStats []record
var eventsMaxLength int
var eventsMutex sync.RWMutex
var eventsλ float64
var eventsBehind int64
var impressionsFlushingStats []record
var impressionsMaxLength int
var impressionsMutex sync.RWMutex
var impressionsλ float64
var impressionsBehind int64

// InitializeEvictionCalculator initializes the eviction calculator module
func InitializeEvictionCalculator() {
	eventsFlushingStats = make([]record, 0)
	eventsMaxLength = int(100) * conf.Data.EventsThreads
	eventsλ = 0
	impressionsFlushingStats = make([]record, 0)
	impressionsMaxLength = int(100) * conf.Data.ImpressionsThreads
	impressionsλ = 0
}

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

func calculateλ(records []record) float64 {
	t := int64(calculateAmountFlushed(records))
	dataInT1 := records[0].DataInStorage
	dataInT2 := records[len(records)-1].DataInStorage
	amountFlushed := int64(calculateAmountFlushed(records))
	return float64(t) / float64(dataInT2-dataInT1+amountFlushed)
}

// StoreDataFlushed stores data flushed
func StoreDataFlushed(timestamp int64, countFlushed int, countInStorage int64, operation string) {
	var newInformation = record{
		Timestamp:     timestamp,
		DataFlushed:   countFlushed,
		DataInStorage: countInStorage,
	}
	if operation == "events" {
		eventsMutex.Lock()
		storeRecord(newInformation, &eventsFlushingStats, eventsMaxLength)
		eventsλ = calculateλ(eventsFlushingStats)
		eventsMutex.Unlock()
	} else {
		impressionsMutex.Lock()
		storeRecord(newInformation, &impressionsFlushingStats, impressionsMaxLength)
		impressionsλ = calculateλ(impressionsFlushingStats)
		impressionsMutex.Unlock()
	}
}

// GetImpressionsλ returns eviction factor for impressions
func GetImpressionsλ() float64 {
	return impressionsλ
}

// GetEventsλ returns eviction factor for events
func GetEventsλ() float64 {
	return eventsλ
}
