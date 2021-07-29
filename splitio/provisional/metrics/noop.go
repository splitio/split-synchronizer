package metrics

import (
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
)

type NoOpTelemetry struct{}

func (t *NoOpTelemetry) RecordConfigData(configData dtos.Config) error            { return nil }
func (t *NoOpTelemetry) RecordNonReadyUsage()                                     {}
func (t *NoOpTelemetry) RecordBURTimeout()                                        {}
func (t *NoOpTelemetry) RecordLatency(method string, latency int64)               {}
func (t *NoOpTelemetry) RecordException(method string)                            {}
func (t *NoOpTelemetry) AddTag(tag string)                                        {}
func (t *NoOpTelemetry) RecordImpressionsStats(dataType int, count int64)         {}
func (t *NoOpTelemetry) RecordEventsStats(dataType int, count int64)              {}
func (t *NoOpTelemetry) RecordSuccessfulSync(resource int, time time.Time)        {}
func (t *NoOpTelemetry) RecordSyncError(resource int, status int)                 {}
func (t *NoOpTelemetry) RecordSyncLatency(resource int, latency time.Duration)    {}
func (t *NoOpTelemetry) RecordAuthRejections()                                    {}
func (t *NoOpTelemetry) RecordTokenRefreshes()                                    {}
func (t *NoOpTelemetry) RecordStreamingEvent(streamingEvent *dtos.StreamingEvent) {}
func (t *NoOpTelemetry) RecordSessionLength(session int64)                        {}
func (t *NoOpTelemetry) GetImpressionsStats(dataType int) int64                   { return 0 }
func (t *NoOpTelemetry) GetEventsStats(dataType int) int64                        { return 0 }
func (t *NoOpTelemetry) GetLastSynchronization() dtos.LastSynchronization {
	return dtos.LastSynchronization{}
}
func (t *NoOpTelemetry) PopHTTPErrors() dtos.HTTPErrors            { return dtos.HTTPErrors{} }
func (t *NoOpTelemetry) PopHTTPLatencies() dtos.HTTPLatencies      { return dtos.HTTPLatencies{} }
func (t *NoOpTelemetry) PopAuthRejections() int64                  { return 0 }
func (t *NoOpTelemetry) PopTokenRefreshes() int64                  { return 0 }
func (t *NoOpTelemetry) PopStreamingEvents() []dtos.StreamingEvent { return []dtos.StreamingEvent{} }
func (t *NoOpTelemetry) PopTags() []string                         { return nil }
func (t *NoOpTelemetry) GetSessionLength() int64                   { return 0 }
