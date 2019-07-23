package task

import (
	"fmt"

	"github.com/splitio/split-synchronizer/conf"
)

type information struct {
	Timestamp     int64
	DataFlushed   int
	DataInStorage int64
}

var eventCounter []information
var eventsMaxLength int
var impressionCounter []information
var impressionsMaxLength int

// InitializeEvictionCalculator appcontext module
func InitializeEvictionCalculator() {
	eventCounter = make([]information, 0)
	eventsMaxLength = int(100) * conf.Data.EventsThreads
	impressionCounter = make([]information, 0)
	impressionsMaxLength = 100 * conf.Data.ImpressionsThreads
}

func storeInformation(info information, counter *[]information, maxLength int) {
	fmt.Println("len(counter)", len(*counter), maxLength)
	if len(*counter) >= maxLength {
		*counter = (*counter)[1:maxLength]
	}
	*counter = append(*counter, info)
}

func calculateAmountFlushed(counter []information) int {
	amountFlushed := 0
	for _, i := range counter {
		amountFlushed += i.DataFlushed
	}
	return amountFlushed
}

func calculateλ(counter []information) float64 {
	t := int64(calculateAmountFlushed(counter))
	dataInT1 := counter[0].DataInStorage
	dataInT2 := counter[len(counter)-1].DataInStorage
	amountFlushed := int64(calculateAmountFlushed(counter))
	fmt.Println("calculateTotalAmountFlushed", t)
	fmt.Println("dataInT1", dataInT1)
	fmt.Println("dataInT2", dataInT2)
	fmt.Println("amountFlushed", amountFlushed)
	return float64(t) / float64(dataInT2-dataInT1+amountFlushed)
}

// StoreDataFlushed stores data flushed
func StoreDataFlushed(timestamp int64, countFlushed int, countInStorage int64, operation string) {
	var newInformation = information{
		Timestamp:     timestamp,
		DataFlushed:   countFlushed,
		DataInStorage: countInStorage,
	}
	if operation == "events" {
		storeInformation(newInformation, &eventCounter, eventsMaxLength)
		fmt.Println("λ:", calculateλ(eventCounter))
		fmt.Println("EVENTS BEHIND:", countInStorage)
	} else {
		storeInformation(newInformation, &impressionCounter, impressionsMaxLength)
		fmt.Println("λ:", calculateλ(impressionCounter))
		fmt.Println("IMPRESSIONS BEHIND:", countInStorage)
	}
}
