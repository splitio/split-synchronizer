package evcalc

import (
	"testing"
	"time"
)

func TestEvictionCalculator(t *testing.T) {
	monitor := New(1)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 0)
	lambda := monitor.Lambda()
	if lambda != 1 {
		t.Error("Lambda should be 1 instead of ", lambda)
	}
}

func TestEvictionCalculatorWithInStorage(t *testing.T) {
	monitor := New(1)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 100)
	lambda := monitor.Lambda()
	if lambda != 1 {
		t.Error("Lambda should be 1 instead of", lambda)
	}

	if len(monitor.flushingStats) != 1 {
		t.Error("It should recorded 1")
	}
}

func TestEvictionCalculatorRegisteringTwo(t *testing.T) {
	monitor := New(1)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 100)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 0)
	lambda := monitor.Lambda()
	if lambda != 2 {
		t.Error("Lambda should be 1 instead of", lambda)
	}

	if len(monitor.flushingStats) != 2 {
		t.Error("It should recorded 2")
	}
}

func TestEvictionCalculatorWithMoreDataThatCanFlush(t *testing.T) {
	monitor := New(1)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 100)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 150)
	monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 200)

	lambda := monitor.Lambda()
	if lambda != 0.75 {
		t.Error("Lambda should be 0.75 instead of", lambda)
	}

	if len(monitor.flushingStats) != 3 {
		t.Error("It should recorded 3")
	}
}

func TestEvictionCalculatorWithMoreDataThatCanFlushAndMoreDataThatCanStore(t *testing.T) {
	monitor := New(1)
	for i := 0; i < 120; i++ {
		monitor.StoreDataFlushed(time.Now().UnixNano(), 100, 100+(int64(i*10)))
	}

	lambda := monitor.Lambda()
	if lambda >= 1 {
		t.Error("Lambda should be less than 1")
	}

	if len(monitor.flushingStats) != 100 {
		t.Error("It should recorded 100")
	}
}
