package tasks

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
)

// recordingLogger captures warnings so tests can assert on drop accounting.
type recordingLogger struct {
	logging.LoggerInterface
	mu       sync.Mutex
	warnings []string
}

func (l *recordingLogger) Warning(msg ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, fmt.Sprint(msg...))
}

func (l *recordingLogger) Debug(msg ...interface{}) {}
func (l *recordingLogger) Info(msg ...interface{})  {}
func (l *recordingLogger) Error(msg ...interface{}) {}

// dropWarnings returns the warnings emitted by flush about dropped messages.
func (l *recordingLogger) dropWarnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, w := range l.warnings {
		if strings.Contains(w, "dropped") {
			out = append(out, w)
		}
	}
	return out
}

// allWarnings returns every recorded warning.
func (l *recordingLogger) allWarnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.warnings...)
}

// newFlushTestTask builds a task with zero workers: nothing drains the worker pool queue,
// so once it is full every further message is dropped by flush.
func newFlushTestTask(queueSize int) (*DeferredRecordingTaskImpl, *recordingLogger) {
	logger := &recordingLogger{}
	return newDeferredFlushTask(logger, nil, 1, queueSize, 0), logger
}

// fillPool pushes n messages and flushes them, leaving the worker pool queue full.
func fillPool(task *DeferredRecordingTaskImpl, n int) {
	for i := 0; i < n; i++ {
		task.queue <- i
	}
	_ = task.flush(task.logger)
}

func TestFlushNoDropsWhenPoolHasRoom(t *testing.T) {
	task, logger := newFlushTestTask(3)
	task.queue <- "a"
	task.queue <- "b"

	if err := task.flush(logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := task.Dropped(); got != 0 {
		t.Errorf("Dropped() = %d, want 0", got)
	}
	if got := logger.dropWarnings(); len(got) != 0 {
		t.Errorf("expected no drop warnings, got %v", got)
	}
	if len(task.queue) != 0 {
		t.Errorf("queue length = %d, want 0", len(task.queue))
	}
}

func TestFlushCountsAndLogsDrops(t *testing.T) {
	task, logger := newFlushTestTask(3)
	fillPool(task, 3)

	task.queue <- "a"
	task.queue <- "b"
	if err := task.flush(logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := task.Dropped(); got != 2 {
		t.Errorf("Dropped() = %d, want 2", got)
	}
	warnings := logger.dropWarnings()
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 drop warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "dropped 2") || !strings.Contains(warnings[0], "total dropped: 2") {
		t.Errorf("unexpected warning text: %q", warnings[0])
	}
	if len(task.queue) != 0 {
		t.Errorf("queue length = %d, want 0", len(task.queue))
	}
}

func TestFlushDropTotalAccumulates(t *testing.T) {
	task, logger := newFlushTestTask(3)
	fillPool(task, 3)

	task.queue <- "a"
	task.queue <- "b"
	_ = task.flush(logger)

	task.queue <- "c"
	_ = task.flush(logger)

	if got := task.Dropped(); got != 3 {
		t.Errorf("Dropped() = %d, want 3", got)
	}
	warnings := logger.dropWarnings()
	if len(warnings) != 2 {
		t.Fatalf("expected 2 drop warnings (one per flush), got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[1], "total dropped: 3") {
		t.Errorf("second warning should carry running total 3, got %q", warnings[1])
	}
}

func TestFlushOneWarningPerFlushNotPerMessage(t *testing.T) {
	task, logger := newFlushTestTask(3)
	fillPool(task, 3)

	task.queue <- "a"
	task.queue <- "b"
	task.queue <- "c"
	_ = task.flush(logger)

	if got := task.Dropped(); got != 3 {
		t.Errorf("Dropped() = %d, want 3", got)
	}
	if got := logger.dropWarnings(); len(got) != 1 {
		t.Errorf("expected exactly 1 drop warning for 3 dropped messages, got %d: %v", len(got), got)
	}
}

func TestFlushSkipsWhenAlreadyRunning(t *testing.T) {
	task, logger := newFlushTestTask(3)
	task.drainInProgress.TestAndSet() // simulate a flush already in progress

	task.queue <- "a"
	task.queue <- "b"
	if err := task.flush(logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(task.queue) != 2 {
		t.Errorf("queue length = %d, want 2 (flush must not drain while another runs)", len(task.queue))
	}
	if got := task.Dropped(); got != 0 {
		t.Errorf("Dropped() = %d, want 0", got)
	}
	found := false
	for _, w := range logger.allWarnings() {
		if strings.Contains(w, "in progress") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the in-progress warning, got %v", logger.allWarnings())
	}
}

func TestFlushReleasesDrainFlag(t *testing.T) {
	task, logger := newFlushTestTask(3)

	task.queue <- "a"
	_ = task.flush(logger)
	task.queue <- "b"
	_ = task.flush(logger)

	if len(task.queue) != 0 {
		t.Errorf("second flush was skipped, queue length = %d, want 0", len(task.queue))
	}
	for _, w := range logger.allWarnings() {
		if strings.Contains(w, "in progress") {
			t.Errorf("drain flag was not released after flush: %v", logger.allWarnings())
		}
	}
}

func TestFlushEmptyQueueIsNoop(t *testing.T) {
	task, logger := newFlushTestTask(3)

	if err := task.flush(logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := task.Dropped(); got != 0 {
		t.Errorf("Dropped() = %d, want 0", got)
	}
	if got := logger.allWarnings(); len(got) != 0 {
		t.Errorf("expected no warnings, got %v", got)
	}
}

func TestFlushConcurrentDropsRace(t *testing.T) {
	const total = 100
	task, logger := newFlushTestTask(3)
	fillPool(task, 3) // pool is full, every message flushed from now on is dropped

	done := make(chan struct{})
	var flushers sync.WaitGroup
	for i := 0; i < 2; i++ {
		flushers.Add(1)
		go func() {
			defer flushers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = task.flush(logger)
				}
			}
		}()
	}

	for i := 0; i < total; i++ {
		task.queue <- i // blocks while the queue is full; flushers drain it
	}
	close(done)
	flushers.Wait()

	// A flush skipped because another was running leaves messages queued; drain them now.
	_ = task.flush(logger)

	if got := task.Dropped(); got != total {
		t.Errorf("Dropped() = %d, want %d", got, total)
	}
}
