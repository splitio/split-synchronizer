package counter

import (
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

func TestThresholdCounter(t *testing.T) {
	counter := NewCounterThresholdImp(Config{
		Name:        "Test",
		CounterType: 0,
		Severity:    0,
		Period:      3,
	}, logging.NewLogger(nil))
	counter.Start()

	counter.NotifyEvent()
	res := counter.IsHealthy()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	time.Sleep(time.Duration(1) * time.Second)

	counter.NotifyEvent()
	res = counter.IsHealthy()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	counter.Reset(1)

	time.Sleep(time.Duration(2) * time.Second)
	res = counter.IsHealthy()
	if res.Healthy {
		t.Errorf("Healthy should be false")
	}

	counter.Stop()
}
