package counter

import (
	"testing"
	"time"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestThresholdCounter(t *testing.T) {
	counter := NewThresholdCounter(&hcCommon.Config{
		Name:        "Test",
		CounterType: 0,
		Severity:    0,
		Period:      3,
		MonitorType: hcCommon.Threshold,
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
