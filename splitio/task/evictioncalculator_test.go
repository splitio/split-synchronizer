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

	delta := GetEventsDelta()
	if delta != 1 {
		t.Error("Delta should be 1 instead of", delta)
	}
}

func TestEvictionCalculatorWithEventsInStorage(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")

	delta := GetEventsDelta()
	if delta != 1 {
		t.Error("Delta should be 1 instead of", delta)
	}

	if len(eventsMonitor.FlushingStats) != 1 {
		t.Error("It should recorded 1")
	}
}

func TestEvictionCalculatorRegisteringTwo(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "events")

	delta := GetEventsDelta()
	if delta != 2 {
		t.Error("Delta should be 1 instead of", delta)
	}

	if len(eventsMonitor.FlushingStats) != 2 {
		t.Error("It should recorded 2")
	}
}

func TestEvictionCalculatorWithMoreEventsThatCanFlush(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 150, "events")
	StoreDataFlushed(time.Now().UnixNano(), 100, 200, "events")

	delta := GetEventsDelta()
	if delta != 0.75 {
		t.Error("Delta should be 0.75 instead of", delta)
	}

	if len(eventsMonitor.FlushingStats) != 3 {
		t.Error("It should recorded 3")
	}
}

func TestEvictionCalculatorWithMoreEventsThatCanFlushAndMoreDataThatCanStore(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	for i := 0; i < 120; i++ {
		StoreDataFlushed(time.Now().UnixNano(), 100, 100+(int64(i*10)), "events")
	}

	delta := GetEventsDelta()
	if delta >= 1 {
		t.Error("Delta should be less than 1")
	}

	if len(eventsMonitor.FlushingStats) != 100 {
		t.Error("It should recorded 100")
	}
}

func TestEvictionCalculatorImpressions(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "impressions")

	delta := GetImpressionsDelta()
	if delta != 1 {
		t.Error("Delta should be 1 instead of", delta)
	}
}

func TestEvictionCalculatorWithImpressionsInStorage(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")

	delta := GetImpressionsDelta()
	if delta != 1 {
		t.Error("Delta should be 1 instead of", delta)
	}

	if len(impressionsMonitor.FlushingStats) != 1 {
		t.Error("It should recorded 1")
	}
}

func TestEvictionCalculatorRegisteringTwoImpressions(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 0, "impressions")

	delta := GetImpressionsDelta()
	if delta != 2 {
		t.Error("Delta should be 1 instead of", delta)
	}

	if len(impressionsMonitor.FlushingStats) != 2 {
		t.Error("It should recorded 2")
	}
}

func TestEvictionCalculatorWithMoreImpressionsThatCanFlush(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	StoreDataFlushed(time.Now().UnixNano(), 100, 100, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 150, "impressions")
	StoreDataFlushed(time.Now().UnixNano(), 100, 200, "impressions")

	delta := GetImpressionsDelta()
	if delta != 0.75 {
		t.Error("Delta should be 0.75 instead of", delta)
	}

	if len(impressionsMonitor.FlushingStats) != 3 {
		t.Error("It should recorded 3")
	}
}

func TestEvictionCalculatorWithMoreImpressionsThatCanFlushAndMoreDataThatCanStore(t *testing.T) {
	conf.Initialize()
	InitializeEvictionCalculator()

	for i := 0; i < 120; i++ {
		StoreDataFlushed(time.Now().UnixNano(), 100, 100+(int64(i*10)), "impressions")
	}

	delta := GetImpressionsDelta()
	if delta >= 1 {
		t.Error("Delta should be less than 1")
	}

	if len(impressionsMonitor.FlushingStats) != 100 {
		t.Error("It should recorded 100")
	}
}
