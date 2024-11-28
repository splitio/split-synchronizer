package conf

import (
	"os"

	"github.com/splitio/go-split-commons/v6/conf"
)

// InitAdvancedOptions initializes an advanced config with default values + overriden urls.
func InitAdvancedOptions(proxy bool) *conf.AdvancedConfig {
	advanced := conf.GetDefaultAdvancedConfig()

	prefix := "SPLIT_SYNC_"
	if proxy {
		prefix = "SPLIT_PROXY_"
		advanced.LargeSegment.Enable = true
	}

	if envSdkURL := os.Getenv(prefix + "SDK_URL"); envSdkURL != "" {
		advanced.SdkURL = envSdkURL
	}

	if envEventsURL := os.Getenv(prefix + "EVENTS_URL"); envEventsURL != "" {
		advanced.EventsURL = envEventsURL
	}

	if authServiceURL := os.Getenv(prefix + "AUTH_SERVICE_URL"); authServiceURL != "" {
		advanced.AuthServiceURL = authServiceURL
	}

	if streamingServiceURL := os.Getenv(prefix + "STREAMING_SERVICE_URL"); streamingServiceURL != "" {
		advanced.StreamingServiceURL = streamingServiceURL
	}

	if telemetryServiceURL := os.Getenv(prefix + "TELEMETRY_SERVICE_URL"); telemetryServiceURL != "" {
		advanced.TelemetryServiceURL = telemetryServiceURL
	}

	return &advanced
}
