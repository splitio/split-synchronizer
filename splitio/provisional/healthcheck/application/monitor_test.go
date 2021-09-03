package application

import (
	"testing"
	"time"

	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/provisional/healthcheck/application/counter"
)

func assertItemsHealthy(t *testing.T, items []ItemDto, splitsExpected bool, segmentsExpected bool, errorsExpected bool) {
	for _, item := range items {
		if item.Name == "Splits" && item.Healthy != splitsExpected {
			t.Errorf("SplitsCounter.Healthy should be %v", splitsExpected)
		}
		if item.Name == "Segments" && item.Healthy != segmentsExpected {
			t.Errorf("SegmentsCounter.Healthy should be %v", segmentsExpected)
		}
		if item.Name == "Sync-Errors" && item.Healthy != errorsExpected {
			t.Errorf("ErrorsCounter.Healthy should be %v", errorsExpected)
		}
	}
}

func TestMonitor(t *testing.T) {
	var cfgs []counter.Config

	splits := counter.Config{
		Name:        "Splits",
		CounterType: counter.Splits,
		Period:      10,
		Severity:    counter.Critical,
	}

	segments := counter.Config{
		Name:        "Segments",
		CounterType: counter.Segments,
		Period:      10,
		Severity:    counter.Critical,
	}

	syncErrors := counter.Config{
		Name:        "Sync-Errors",
		CounterType: counter.SyncErros,
		Period:      10,
		Periodic:    true,
		TaskFunc: func(l logging.LoggerInterface, c counter.BaseCounterInterface) error {
			if c.IsHealthy().Healthy {
				c.Reset(0)
			}

			return nil
		},
		MaxErrorsAllowedInPeriod: 1,
		Severity:                 counter.Low,
	}

	cfgs = append(cfgs, splits, segments, syncErrors)

	monitor := NewMonitorImp(cfgs, logging.NewLogger(nil))

	monitor.Start()

	monitor.NotifyEvent(counter.SyncErros)
	monitor.NotifyEvent(counter.SyncErros)
	monitor.NotifyEvent(counter.SyncErros)
	res := monitor.GetHealthStatus()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	assertItemsHealthy(t, res.Items, true, true, false)

	monitor.NotifyEvent(counter.Splits)
	monitor.NotifyEvent(counter.Segments)

	res = monitor.GetHealthStatus()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	assertItemsHealthy(t, res.Items, true, true, false)

	monitor.Reset(counter.Splits, 1)

	time.Sleep(time.Duration(2) * time.Second)
	res = monitor.GetHealthStatus()
	if res.Healthy {
		t.Errorf("Healthy should be false")
	}

	assertItemsHealthy(t, res.Items, false, true, false)
	monitor.Stop()
}
