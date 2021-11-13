package services

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services/counter"
)

func TestGetHealthStatusByPercentage(t *testing.T) {
	var serviceCounters []counter.ServicesCounterInterface

	eventsConfig := counter.Config{
		Name:                  "EVENTS",
		ServiceURL:            "https://events.test.io/api",
		ServiceHealthEndpoint: "/version",
		TaskPeriod:            100,
		MaxLen:                2,
		PercentageToBeHealthy: 100,
		Severity:              counter.Critical,
	}

	criticalCounter := counter.NewCounterByPercentage(eventsConfig, logging.NewLogger(nil))

	streamingConfig := counter.Config{
		Name:                  "STREAMING",
		ServiceURL:            "https://streaming.test.io",
		ServiceHealthEndpoint: "/health",
		TaskPeriod:            100,
		MaxLen:                2,
		PercentageToBeHealthy: 100,
		Severity:              counter.Degraded,
	}

	degradedCounter := counter.NewCounterByPercentage(streamingConfig, logging.NewLogger(nil))

	serviceCounters = append(serviceCounters, criticalCounter, degradedCounter)

	m := MonitorImp{
		Counters: serviceCounters,
	}

	res := m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyHit(500, "message error")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded")
	}

	criticalCounter.NotifyHit(500, "message error")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down")
	}

	criticalCounter.NotifyHit(200, "")
	criticalCounter.NotifyHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}
}
