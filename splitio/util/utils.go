package util

import (
	"fmt"
	"os"
	"time"

	"github.com/splitio/go-split-commons/conf"
	sync "github.com/splitio/split-synchronizer/conf"
)

// ParseTime parses a date to format d h m s
func ParseTime(date time.Time) string {
	upt := time.Since(date)
	d := int64(0)
	h := int64(0)
	m := int64(0)
	s := int64(upt.Seconds())

	if s > 60 {
		m = int64(s / 60)
		s = s - m*60
	}

	if m > 60 {
		h = int64(m / 60)
		m = m - h*60
	}

	if h > 24 {
		d = int64(h / 24)
		h = h - d*24
	}

	return fmt.Sprintf("%dd %dh %dm %ds", d, h, m, s)
}

// ParseAdvancedOptions parses defaults for advanced Options
func ParseAdvancedOptions() conf.AdvancedConfig {
	advanced := conf.GetDefaultAdvancedConfig()
	advanced.EventsBulkSize = sync.Data.EventsPerPost
	advanced.HTTPTimeout = int(sync.Data.HTTPTimeout)
	advanced.ImpressionsBulkSize = sync.Data.ImpressionsPerPost
	// EventsQueueSize:      5000, // MISSING
	// ImpressionsQueueSize: 5000, // MISSING
	// SegmentQueueSize:     100,  // MISSING
	// SegmentWorkers:       10,   // MISSING

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
