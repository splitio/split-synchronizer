package interfaces

import (
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/util"
)

// TelemetryStorage storage
var TelemetryStorage *mutexmap.MMMetricsStorage

// ProxyTelemetryWrapper telemetry
var ProxyTelemetryWrapper *storage.MetricWrapper

// MetricsRecorder recorder
var MetricsRecorder *api.HTTPMetricsRecorder

// Initialize Wrappers
func Initialize() {
	if TelemetryStorage == nil {
		TelemetryStorage = mutexmap.NewMMMetricsStorage()
	}

	if ProxyTelemetryWrapper == nil {
		ProxyTelemetryWrapper = &storage.MetricWrapper{
			LocalTelemtry: mutexmap.NewMMMetricsStorage(),
			Telemetry:     TelemetryStorage,
			Logger:        log.Instance,
		}
	}

	if MetricsRecorder == nil {
		MetricsRecorder = api.NewHTTPMetricsRecorder(conf.Data.APIKey, util.ParseAdvancedOptions(), log.Instance)
	}
}
