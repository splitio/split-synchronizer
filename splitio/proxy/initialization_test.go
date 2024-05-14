package proxy

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v6/synchronizer"
)

type syncManagerMock struct {
	c         chan int
	execCount int64
}

func (m *syncManagerMock) IsRunning() bool { panic("unimplemented") }
func (m *syncManagerMock) Start() {
	atomic.AddInt64(&m.execCount, 1)
	switch atomic.LoadInt64(&m.execCount) {
	case 1:
		m.c <- synchronizer.Error
	default:
		m.c <- synchronizer.Ready
	}
}
func (m *syncManagerMock) Stop() { panic("unimplemented") }

var _ synchronizer.Manager = (*syncManagerMock)(nil)

func TestSyncManagerInitializationRetriesWithSnapshot(t *testing.T) {

	sm := &syncManagerMock{c: make(chan int, 1)}

	// No snapshot and error
	complete := make(chan struct{}, 1)
	err := startBGSyng(sm, sm.c, false, func() { complete <- struct{}{} })
	if err != errUnrecoverable {
		t.Error("should be an unrecoverable error. Got: ", err)
	}

	select {
	case <-complete:
		t.Error("nothing should be published on the channel")
	case <-time.After(500 * time.Millisecond):
		// all good
	}

	// Snapshot and error
	atomic.StoreInt64(&sm.execCount, 0)
	err = startBGSyng(sm, sm.c, true, func() { complete <- struct{}{} })
	if err != errRetrying {
		t.Error("should be a retrying error. Got: ", err)
	}

	select {
	case <-complete:
		// all good
	case <-time.After(2500 * time.Millisecond):
		t.Error("should not time out")
	}

	if atomic.LoadInt64(&sm.execCount) != 2 {
		t.Error("there should be 2 executions")
	}
}
