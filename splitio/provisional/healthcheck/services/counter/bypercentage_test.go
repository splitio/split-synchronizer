package counter

import (
	"container/list"
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
)

func TestNotifyServiceHitByPercentage(t *testing.T) {
	c := ByPercentageImp{
		BaseCounterImp:        *NewBaseCounterImp("TestCounter", 1, logging.NewLogger(nil)),
		maxLen:                6,
		cache:                 new(list.List),
		percentageToBeHealthy: 60,
	}

	res := c.IsHealthy()
	if res.Healthy != true {
		t.Errorf("Health should be true")
	}

	if res.LastMessage != "" {
		t.Errorf("LastMessage should be empty")
	}

	if res.Severity != 1 {
		t.Errorf("Severity should be 1")
	}

	c.NotifyServiceHit(500, "Error-1")
	res = c.IsHealthy()
	if res.Healthy != false {
		t.Errorf("Health should be false")
	}

	if res.LastMessage != "Error-1" {
		t.Errorf("LastMessage should be Error-1")
	}

	c.NotifyServiceHit(500, "Error-2")
	res = c.IsHealthy()
	if res.Healthy != false {
		t.Errorf("Health should be false")
	}

	if res.LastMessage != "Error-2" {
		t.Errorf("LastMessage should be Error-2")
	}

	c.NotifyServiceHit(200, "")
	c.NotifyServiceHit(200, "")
	res = c.IsHealthy()
	if res.Healthy != false {
		t.Errorf("Health should be false")
	}

	if res.LastMessage != "Error-2" {
		t.Errorf("LastMessage should be Error-2")
	}

	c.NotifyServiceHit(200, "")
	res = c.IsHealthy()
	if res.Healthy != true {
		t.Errorf("Health should be true")
	}

	if res.LastMessage != "" {
		t.Errorf("LastMessage should be empty. %s", res.LastMessage)
	}
}
