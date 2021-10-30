package conf

import (
	"os"

	"github.com/splitio/go-split-commons/v4/conf"
)

// InitAdvancedOptions initializes an advanced config with default values + overriden urls.
func InitAdvancedOptions() *conf.AdvancedConfig {

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

	streamingServiceURL := os.Getenv("SPLITIO_STREAMING_SERVICE_URL")
	if streamingServiceURL != "" {
		advanced.StreamingServiceURL = streamingServiceURL
	}

	telemetryServiceURL := os.Getenv("SPLITIO_TELEMETRY_SERVICE_URL")
	if telemetryServiceURL != "" {
		advanced.TelemetryServiceURL = telemetryServiceURL
	}

	return &advanced
}
