package services

import (
	"testing"

	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/services"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/services/counter"
)

func TestGetHealthStatusByPercentage(t *testing.T) {
	var serviceCounters []counter.BaseCounterInterface

	eventsConfig := counter.Config{
		Name:                  "EVENTS",
		ServiceURL:            "https://events.test.io/api",
		ServiceHealthEndpoint: "/version",
		TaskPeriod:            100,
		CounterType:           hcCommon.ByPercentage,
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
		CounterType:           hcCommon.ByPercentage,
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

	degradedCounter.NotifyServiceHit(500, "message error")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded")
	}

	criticalCounter.NotifyServiceHit(500, "message error")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down")
	}

	criticalCounter.NotifyServiceHit(200, "")
	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != degradedStatus {
		t.Errorf("Status should be degraded - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	degradedCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}
}

func TestGetHealthStatusSecuencial(t *testing.T) {
	var serviceCounters []counter.BaseCounterInterface

	eventsConfig := counter.Config{
		Name:                  "EVENTS",
		ServiceURL:            "https://events.test.io/api",
		ServiceHealthEndpoint: "/version",
		TaskPeriod:            100,
		CounterType:           hcCommon.Sequential,
		Severity:              counter.Critical,
		MaxErrorsAllowed:      3,
		MinSuccessExpected:    5,
	}
	criticalCounter := counter.NewCounterSecuencial(eventsConfig, logging.NewLogger(nil))

	streamingConfig := counter.Config{
		Name:                  "STREAMING",
		ServiceURL:            "https://streaming.test.io",
		ServiceHealthEndpoint: "/health",
		TaskPeriod:            100,
		CounterType:           hcCommon.Sequential,
		MaxLen:                2,
		PercentageToBeHealthy: 100,
		Severity:              counter.Degraded,
		MaxErrorsAllowed:      3,
		MinSuccessExpected:    5,
	}
	degradedCounter := counter.NewCounterSecuencial(streamingConfig, logging.NewLogger(nil))

	serviceCounters = append(serviceCounters, criticalCounter, degradedCounter)

	m := MonitorImp{
		Counters: serviceCounters,
	}

	res := m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(500, "Error 1")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(500, "Error 1")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(500, "Error 1")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != downStatus {
		t.Errorf("Status should be down - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}

	criticalCounter.NotifyServiceHit(500, "error 2")
	criticalCounter.NotifyServiceHit(500, "error 3")
	criticalCounter.NotifyServiceHit(200, "")

	res = m.GetHealthStatus()

	if res.Status != healthyStatus {
		t.Errorf("Status should be healthy - Actual status: %s", res.Status)
	}
}
