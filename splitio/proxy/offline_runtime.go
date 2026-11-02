package proxy

import (
	"errors"
	"time"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/service"
	"github.com/splitio/go-split-commons/v10/synchronizer"
	hcApplication "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
	hcServices "github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services"
)

// idleSyncManager satisfies the runtime shutdown path without starting sync.
type idleSyncManager struct{}

func (idleSyncManager) Start() {}

func (idleSyncManager) Stop() {}

func (idleSyncManager) IsRunning() bool { return false }

func (idleSyncManager) StartBGSync(chan int, bool, func()) error { return nil }

var _ synchronizer.Manager = idleSyncManager{}

// staticAppMonitor reports application health without refresh timers.
type staticAppMonitor struct {
	since time.Time
}

func newStaticAppMonitor() *staticAppMonitor {
	return &staticAppMonitor{since: time.Now()}
}

func (m *staticAppMonitor) GetHealthStatus() hcApplication.HealthDto {
	since := m.since
	return hcApplication.HealthDto{
		Healthy:      true,
		HealthySince: &since,
		Items:        []hcApplication.ItemDto{},
	}
}

func (m *staticAppMonitor) NotifyEvent(int) {}

func (m *staticAppMonitor) Reset(int, int) {}

func (m *staticAppMonitor) Start() {}

func (m *staticAppMonitor) Stop() {}

var _ hcApplication.MonitorIterface = (*staticAppMonitor)(nil)

// staticServicesMonitor reports dependency checks as disabled and healthy.
// The health handler dereferences this monitor, so it cannot be nil.
type staticServicesMonitor struct {
	since time.Time
}

func newStaticServicesMonitor() *staticServicesMonitor {
	return &staticServicesMonitor{since: time.Now()}
}

func (m *staticServicesMonitor) GetHealthStatus() hcServices.HealthDto {
	since := m.since
	return hcServices.HealthDto{
		Status: "healthy",
		Items: []hcServices.ItemDto{{
			Service:      "cloud",
			Healthy:      true,
			Message:      "cloud checks disabled",
			HealthySince: &since,
		}},
	}
}

func (m *staticServicesMonitor) Start() {}

func (m *staticServicesMonitor) Stop() {}

var _ hcServices.MonitorIterface = (*staticServicesMonitor)(nil)

// offlineSplitFetcher refuses any fetch so a stale since cannot dial FME.
type offlineSplitFetcher struct{}

func (offlineSplitFetcher) Fetch(*service.FlagRequestParams) (dtos.FFResponse, error) {
	return nil, errors.New("split proxy offline mode does not contact FME")
}

func (offlineSplitFetcher) IsProxy() bool { return true }

var _ service.SplitFetcher = offlineSplitFetcher{}
