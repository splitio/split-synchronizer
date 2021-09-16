package application

import (
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/healthcheck/application"
	hcCommon "github.com/splitio/go-split-commons/v4/healthcheck/application"
	"github.com/splitio/go-toolkit/v5/logging"
)

func assertItemsHealthy(t *testing.T, items []hcCommon.ItemDto, splitsExpected bool, segmentsExpected bool, errorsExpected bool) {
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
	var cfgs []*hcCommon.Config

	splits := &hcCommon.Config{
		Name:        "Splits",
		CounterType: application.Splits,
		Period:      10,
		Severity:    hcCommon.Critical,
	}

	segments := &hcCommon.Config{
		Name:        "Segments",
		CounterType: application.Segments,
		Period:      10,
		Severity:    hcCommon.Critical,
	}

	syncErrors := &hcCommon.Config{
		Name:        "Sync-Errors",
		CounterType: application.SyncErros,
		Period:      10,
		TaskFunc: func(l logging.LoggerInterface, c hcCommon.CounterInterface) error {
			if c.IsHealthy().Healthy {
				c.Reset(0)
			}

			return nil
		},
		MaxErrorsAllowedInPeriod: 1,
		Severity:                 hcCommon.Low,
	}

	cfgs = append(cfgs, splits, segments, syncErrors)

	monitor := NewMonitorImp(cfgs, logging.NewLogger(nil))

	monitor.Start()

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
