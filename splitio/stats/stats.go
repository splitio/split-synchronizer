package stats

import (
	"time"

	"github.com/splitio/split-synchronizer/splitio/util"
)

var startTime time.Time

func init() {
	startTime = time.Now()
}

// Uptime returns a time.Duration since startTIme
func Uptime() time.Duration {
	return time.Since(startTime)
}

// UptimeFormatted formats uptime for humman read
func UptimeFormatted() string {
	return util.ParseTime(startTime)
}
