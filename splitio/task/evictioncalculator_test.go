package task

import (
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/conf"
)

func TestEvictionCalculatorEvents(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "events")

	λ := GetEventsλ()
	if λ != 1 {
		t.Error("λ should be 1 instead of", λ)
	}
}

func TestEvictionCalculatorWithEventsInStorage(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")

	λ := GetEventsλ()
	if λ != 1 {
		t.Error("λ should be 1 instead of", λ)
	}

	if len(eventsFlushingStats) != 1 {
		t.Error("It should recorded 1")
	}
}

func TestEvictionCalculatorRegisteringTwo(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "events")

	λ := GetEventsλ()
	if λ != 2 {
		t.Error("λ should be 1 instead of", λ)
	}

	if len(eventsFlushingStats) != 2 {
		t.Error("It should recorded 2")
	}
}

func TestEvictionCalculatorWithMoreEventsThatCanFlush(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 150, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 200, "events")

	λ := GetEventsλ()
	if λ != 0.75 {
		t.Error("λ should be 0.75 instead of", λ)
	}

	if len(eventsFlushingStats) != 3 {
		t.Error("It should recorded 3")
	}
}

func TestEvictionCalculatorWithMoreEventsThatCanFlushAndMoreDataThatCanStore(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	for i := 0; i < 120; i++ {
		StoreDataFlushed(time.Now().UnixNano(), 100, 100+(int64(i*10)), "events")
	}

	λ := GetEventsλ()
	if λ >= 1 {
		t.Error("λ should be less than 1")
	}

	if len(eventsFlushingStats) != 100 {
		t.Error("It should recorded 100")
	}
}

func TestEvictionCalculatorImpressions(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "impressions")

	λ := GetImpressionsλ()
	if λ != 1 {
		t.Error("λ should be 1 instead of", λ)
	}
}

func TestEvictionCalculatorWithImpressionsInStorage(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")

	λ := GetImpressionsλ()
	if λ != 1 {
		t.Error("λ should be 1 instead of", λ)
	}

	if len(impressionsFlushingStats) != 1 {
		t.Error("It should recorded 1")
	}
}

func TestEvictionCalculatorRegisteringTwoImpressions(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "impressions")

	λ := GetImpressionsλ()
	if λ != 2 {
		t.Error("λ should be 1 instead of", λ)
	}

	if len(impressionsFlushingStats) != 2 {
		t.Error("It should recorded 2")
	}
}

func TestEvictionCalculatorWithMoreImpressionsThatCanFlush(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 150, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 200, "impressions")

	λ := GetImpressionsλ()
	if λ != 0.75 {
		t.Error("λ should be 0.75 instead of", λ)
	}

	if len(impressionsFlushingStats) != 3 {
		t.Error("It should recorded 3")
	}
}

func TestEvictionCalculatorWithMoreImpressionsThatCanFlushAndMoreDataThatCanStore(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	for i := 0; i < 120; i++ {
		StoreDataFlushed(time.Now().UnixNano(), 100, 100+(int64(i*10)), "impressions")
	}

	λ := GetImpressionsλ()
	if λ >= 1 {
		t.Error("λ should be less than 1")
	}

	if len(impressionsFlushingStats) != 100 {
		t.Error("It should recorded 100")
	}
}
