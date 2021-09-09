package counter

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
)

func TestNotifyServiceHitSecuencial(t *testing.T) {
	c := SecuencialImp{
		BaseCounterImp:     *NewBaseCounterImp("TestCounter", 0, logging.NewLogger(nil)),
		maxErrorsAllowed:   2,
		minSuccessExpected: 3,
	}

	res := c.IsHealthy()
	if res.Healthy != true {
		t.Errorf("Health should be true")
	}

	if res.LastMessage != "" {
		t.Errorf("LastMessage should be empty")
	}

	if res.Severity != 0 {
		t.Errorf("Severity should be 0")
	}

	c.NotifyServiceHit(500, "Error-1")
	c.NotifyServiceHit(500, "Error-2")
	res = c.IsHealthy()

	if res.Healthy != false {
		t.Errorf("Health should be false")
	}

	if res.LastMessage != "Error-2" {
		t.Errorf("LastMessage should be Error-2. Actual message: %s", res.LastMessage)
	}

	c.NotifyServiceHit(200, "")

	res = c.IsHealthy()
	if res.Healthy != false {
		t.Errorf("Health should be false")
	}

	if res.LastMessage != "Error-2" {
		t.Errorf("LastMessage should be Error-2. Actual message: %s", res.LastMessage)
	}

	c.NotifyServiceHit(200, "")
	c.NotifyServiceHit(200, "")
	res = c.IsHealthy()

	if res.Healthy != true {
		t.Errorf("Health should be true")
	}

	if res.LastMessage != "" {
		t.Errorf("LastMessage should be empty")
	}
}
