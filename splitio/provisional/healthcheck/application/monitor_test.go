package application

import (
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/logging"
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
	splitsCfg := counter.ThresholdConfig{
		Name:     "Splits",
		Period:   10,
		Severity: counter.Critical,
	}

	segmentsCfg := counter.ThresholdConfig{
		Name:     "Segments",
		Period:   10,
		Severity: counter.Critical,
	}

	syncErrorsCfg := counter.PeriodicConfig{
		Name:   "Sync-Errors",
		Period: 10,
		TaskFunc: func(l logging.LoggerInterface, c counter.PeriodicCounterInterface) error {
			if c.IsHealthy().Healthy {
				c.ResetErrorCount(0)
			}

			return nil
		},
		MaxErrorsAllowedInPeriod: 1,
		Severity:                 counter.Low,
		ValidationFunc: func(c counter.PeriodicCounterInterface) {
			c.NotifyError()
		},
	}

	monitor := NewMonitorImp(splitsCfg, segmentsCfg, &syncErrorsCfg, logging.NewLogger(nil))

	monitor.Start()

	time.Sleep(time.Duration(1) * time.Second)

	monitor.NotifyEvent(application.SyncErros)
	monitor.NotifyEvent(application.SyncErros)
	monitor.NotifyEvent(application.SyncErros)
	res := monitor.GetHealthStatus()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	assertItemsHealthy(t, res.Items, true, true, false)

	monitor.NotifyEvent(application.Splits)
	monitor.NotifyEvent(application.Segments)

	res = monitor.GetHealthStatus()
	if !res.Healthy {
		t.Errorf("Healthy should be true")
	}

	assertItemsHealthy(t, res.Items, true, true, false)

	monitor.Reset(application.Splits, 1)

	time.Sleep(time.Duration(2) * time.Second)
	res = monitor.GetHealthStatus()
	if res.Healthy {
		t.Errorf("Healthy should be false")
	}

	assertItemsHealthy(t, res.Items, false, true, false)
	monitor.Stop()
}
