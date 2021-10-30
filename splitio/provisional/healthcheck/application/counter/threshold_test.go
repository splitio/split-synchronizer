package counter

import (
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

func TestThresholdCounter(t *testing.T) {
	counter := NewThresholdCounter(ThresholdConfig{
		Name:     "Test",
		Severity: 0,
		Period:   3,
	}, logging.NewLogger(nil))
	counter.Start()

	counter.NotifyHit()
	res := counter.IsHealthy()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	time.Sleep(time.Duration(1) * time.Second)

	counter.NotifyHit()
	res = counter.IsHealthy()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	counter.ResetThreshold(1)

	time.Sleep(time.Duration(2) * time.Second)
	res = counter.IsHealthy()
	if res.Healthy {
		t.Errorf("Healthy should be false")
	}

	counter.Stop()
}
