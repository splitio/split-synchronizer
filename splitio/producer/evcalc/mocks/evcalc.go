package mocks

import "time"

type EvCalcMock struct {
	StoreDataFlushedCall func(timestamp time.Time, countFlushed int, countInStorage int64)
	LambdaCall           func() float64
	AcquireCall          func() bool
	ReleaseCall          func()
	BusyCall             func() bool
}

func (e *EvCalcMock) StoreDataFlushed(timestamp time.Time, countFlushed int, countInStorage int64) {
	e.StoreDataFlushedCall(timestamp, countFlushed, countInStorage)
}

func (e *EvCalcMock) Lambda() float64 {
	return e.LambdaCall()
}

func (e *EvCalcMock) Acquire() bool {
	return e.AcquireCall()
}

func (e *EvCalcMock) Release() {
	e.ReleaseCall()
}

func (e *EvCalcMock) Busy() bool {
	return e.BusyCall()
}
