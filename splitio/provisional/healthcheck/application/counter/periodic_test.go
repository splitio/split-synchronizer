package counter

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
)

func TestPeriodicCounter(t *testing.T) {

	steps := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	counter := NewPeriodicCounter(PeriodicConfig{
		Name:                     "Test",
		Period:                   2,
		MaxErrorsAllowedInPeriod: 2,
		Severity:                 0,
		ValidationFunc: func(c PeriodicCounterInterface) {
			<-steps
			c.NotifyError()
			done <- struct{}{}
			<-steps
			c.NotifyError()
			done <- struct{}{}
		},
	}, logging.NewLogger(nil))

	counter.Start()

	if res := counter.IsHealthy(); !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	steps <- struct{}{}
	<-done
	if res := counter.IsHealthy(); !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	steps <- struct{}{}
	<-done
	if res := counter.IsHealthy(); res.Healthy {
		t.Errorf("Healthy should be false")
	}

	counter.lock.RLock()
	if counter.errorCount != 2 {
		t.Errorf("Errors should be 2")
	}
	counter.lock.RUnlock()

	counter.resetErrorCount()

	if res := counter.IsHealthy(); !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	counter.lock.RLock()
	if counter.errorCount != 0 {
		t.Errorf("Errors should be 0")
	}
	counter.lock.RUnlock()
	counter.Stop()
}
