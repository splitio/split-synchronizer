package interfaces

import (
	"log"
	"os"

	"github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/storage/mutexmap"
	"github.com/splitio/go-toolkit/logging"
)

// GetAdvancedConfig s
func GetAdvancedConfig() conf.AdvancedConfig {
	advanced := conf.GetDefaultAdvancedConfig()
	envSdkURL := os.Getenv("SPLITIO_SDK_URL")
	if envSdkURL != "" {
		advanced.SdkURL = envSdkURL
	} else {
		advanced.SdkURL = "https://sdk.split.io/api"
	}

	envEventsURL := os.Getenv("SPLITIO_EVENTS_URL")
	if envEventsURL != "" {
		advanced.EventsURL = envEventsURL
	} else {
		advanced.EventsURL = "https://events.split.io/api"
	}
	return advanced
}

// TelemetryStorage storage
var TelemetryStorage *mutexmap.MMMetricsStorage = mutexmap.NewMMMetricsStorage()

// ProxyTelemetryWrapper telemetry
var ProxyTelemetryWrapper storage.MetricWrapper = storage.MetricWrapper{
	LocalTelemtry: mutexmap.NewMMMetricsStorage(),
	Telemetry:     TelemetryStorage,
	Logger:        nil,
}

// Logger logger
var Logger = logging.NewLogger(&logging.LoggerOptions{
	StandardLoggerFlags: log.LUTC | log.Ldate | log.Lmicroseconds | log.Lshortfile,
	LogLevel:            logging.LevelInfo,
})
