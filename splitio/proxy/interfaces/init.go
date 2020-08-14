package interfaces

import (
	"os"

	"github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	"github.com/splitio/split-synchronizer/log"
)

// GetAdvancedConfig s
func GetAdvancedConfig() conf.AdvancedConfig {
	advanced := conf.GetDefaultAdvancedConfig()
	envSdkURL := os.Getenv("SPLITIO_SDK_URL")
	if envSdkURL != "" {
		advanced.SdkURL = envSdkURL
	}

	envEventsURL := os.Getenv("SPLITIO_EVENTS_URL")
	if envEventsURL != "" {
		advanced.EventsURL = envEventsURL
	}

	authServiceURL := os.Getenv("SPLITIO_AUTH_SERVICE_URL")
	if authServiceURL != "" {
		advanced.AuthServiceURL = authServiceURL
	}

	return advanced
}

// TelemetryStorage storage
var TelemetryStorage *mutexmap.MMMetricsStorage = mutexmap.NewMMMetricsStorage()

// ProxyTelemetryWrapper telemetry
var ProxyTelemetryWrapper storage.MetricWrapper = storage.MetricWrapper{
	LocalTelemtry: mutexmap.NewMMMetricsStorage(),
	Telemetry:     TelemetryStorage,
	Logger:        log.Instance,
}
