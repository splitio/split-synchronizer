package interfaces

import (
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	"github.com/splitio/split-synchronizer/log"
)

// TelemetryStorage storage
var TelemetryStorage *mutexmap.MMMetricsStorage = mutexmap.NewMMMetricsStorage()

// ProxyTelemetryWrapper telemetry
var ProxyTelemetryWrapper storage.MetricWrapper = storage.MetricWrapper{
	LocalTelemtry: mutexmap.NewMMMetricsStorage(),
	Telemetry:     TelemetryStorage,
	Logger:        log.Instance,
}
