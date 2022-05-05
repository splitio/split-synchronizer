package storage

import (
	"sort"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/v4/storage"
)

// Granularity selection constants to be used upon component instantiation
const (
	HistoricTelemetryGranularityMinute = iota
	HistoricTelemetryGranularityHour
	HistoricTelemetryGranularityDay
)

// TimeslicedProxyEndpointTelemetry is a proxy telemetry facade (yet another) that bundles global data
// and historic data by timeslice (for observability purposes)
type TimeslicedProxyEndpointTelemetry interface {
	ProxyTelemetryFacade
	TimeslicedReport() TimeSliceData
	TotalMetricsReport() map[string]ForResource
}

// TimeslicedProxyEndpointTelemetryImpl is an implementation of `TimeslicedProxyEnxpointTelemetry`
type TimeslicedProxyEndpointTelemetryImpl struct {
	ProxyTelemetryFacade
	telemetryByTimeSlice telemetryByTimeSlice
	timeSliceWidth       int64
	maxTimeSlices        int
	mutex                sync.Mutex
	clock                clock // this is just to be able to mock the time and do proper unit testing
}

// NewTimeslicedProxyEndpointTelemetry constructs a new timesliced proxy-endpoint telemetry
func NewTimeslicedProxyEndpointTelemetry(wrapped ProxyTelemetryFacade, width int64, maxTimeSlices int) *TimeslicedProxyEndpointTelemetryImpl {
	return &TimeslicedProxyEndpointTelemetryImpl{
		ProxyTelemetryFacade: wrapped,
		telemetryByTimeSlice: make(telemetryByTimeSlice),
		timeSliceWidth:       width,
		maxTimeSlices:        maxTimeSlices,
		clock:                &sysClock{},
	}
}

func (t *TimeslicedProxyEndpointTelemetryImpl) TotalMetricsReport() map[string]ForResource {
	return map[string]ForResource{
		"auth":                   newForResource(t.PeekEndpointLatency(AuthEndpoint), t.PeekEndpointStatus(AuthEndpoint)),
		"splitChanges":           newForResource(t.PeekEndpointLatency(SplitChangesEndpoint), t.PeekEndpointStatus(SplitChangesEndpoint)),
		"segmentChanges":         newForResource(t.PeekEndpointLatency(SegmentChangesEndpoint), t.PeekEndpointStatus(SegmentChangesEndpoint)),
		"mySegments":             newForResource(t.PeekEndpointLatency(MySegmentsEndpoint), t.PeekEndpointStatus(MySegmentsEndpoint)),
		"impressionsBulk":        newForResource(t.PeekEndpointLatency(ImpressionsBulkEndpoint), t.PeekEndpointStatus(ImpressionsBulkEndpoint)),
		"impressionsBulkBeacon":  newForResource(t.PeekEndpointLatency(ImpressionsBulkBeaconEndpoint), t.PeekEndpointStatus(ImpressionsBulkBeaconEndpoint)),
		"impressionsCount":       newForResource(t.PeekEndpointLatency(ImpressionsCountEndpoint), t.PeekEndpointStatus(ImpressionsCountEndpoint)),
		"impressionsCountBeacon": newForResource(t.PeekEndpointLatency(ImpressionsCountBeaconEndpoint), t.PeekEndpointStatus(ImpressionsCountBeaconEndpoint)),
		"eventsBulk":             newForResource(t.PeekEndpointLatency(EventsBulkEndpoint), t.PeekEndpointStatus(EventsBulkEndpoint)),
		"eventsBulkBeacon":       newForResource(t.PeekEndpointLatency(EventsBulkBeaconEndpoint), t.PeekEndpointStatus(EventsBulkBeaconEndpoint)),
		"telemetryConfig":        newForResource(t.PeekEndpointLatency(TelemetryConfigEndpoint), t.PeekEndpointStatus(TelemetryConfigEndpoint)),
		"telemetryRuntime":       newForResource(t.PeekEndpointLatency(TelemetryRuntimeEndpoint), t.PeekEndpointStatus(TelemetryRuntimeEndpoint)),
	}
}

// TimeslicedReport returns a report of the latest metrics split into N time-slices
func (t *TimeslicedProxyEndpointTelemetryImpl) TimeslicedReport() TimeSliceData {
	// gather the data
	t.mutex.Lock()
	data := make([]*timeSliceTelemetry, 0, len(t.telemetryByTimeSlice))
	for _, v := range t.telemetryByTimeSlice {
		if v != nil { // should always be true but still...
			data = append(data, v)
		}
	}
	t.mutex.Unlock()

	return formatTimeSeriesData(data)
}

// RecordEndpointLatency increments the latency bucket for a specific endpoint (global + historic records are updated)
func (t *TimeslicedProxyEndpointTelemetryImpl) RecordEndpointLatency(endpoint int, latency time.Duration) {
	t.ProxyTelemetryFacade.RecordEndpointLatency(endpoint, latency)
	timesliced := t.geHistoricForTS(t.clock.Now())
	timesliced.latencies.RecordEndpointLatency(endpoint, latency)
}

// IncrEndpointStatus increments the status code count for a specific endpont/status code (global + historic records are updated)
func (t *TimeslicedProxyEndpointTelemetryImpl) IncrEndpointStatus(endpoint int, status int) {
	t.ProxyTelemetryFacade.IncrEndpointStatus(endpoint, status)
	timesliced := t.geHistoricForTS(t.clock.Now())
	timesliced.statusCodes.IncrEndpointStatus(endpoint, status)
}

func (t *TimeslicedProxyEndpointTelemetryImpl) geHistoricForTS(ts time.Time) *timeSliceTelemetry {
	timeSlice := keyForTimeSlice(ts, t.timeSliceWidth)

	// The following critical section guards access to the timeslice -> telemetry map AND
	// the rollover mechanism if a new entry is created and the count is greater than the allowed max.
	// `EndpointStatusCodes & `ProxyEndpointLatencies` structs have their own synchronization mechanisms
	// and are safe to use by the the reference is returned
	t.mutex.Lock()
	current, ok := t.telemetryByTimeSlice[timeSlice]
	if !ok {
		current = newTimeSliceTelemetry(timeSlice)
		t.telemetryByTimeSlice[timeSlice] = current
		if len(t.telemetryByTimeSlice) > t.maxTimeSlices {
			t.unsafeRollover()
		}
	}
	t.mutex.Unlock()
	return current
}

// warning: This method is meant to be called from `getHistoricForTS` whenever needed WITH THE LOCK ACQUIRED. Otherwise it may crash the app
func (t *TimeslicedProxyEndpointTelemetryImpl) unsafeRollover() {
	if len(t.telemetryByTimeSlice) <= t.maxTimeSlices {
		return // we're within boundaries, nothing to do here
	}

	keys := make([]int64, 0, len(t.telemetryByTimeSlice))
	for key := range t.telemetryByTimeSlice {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, key := range keys[0:(len(keys) - t.maxTimeSlices)] { // narrow view of the slice only contain older elements to be deleted
		delete(t.telemetryByTimeSlice, key)
	}
}

type telemetryByTimeSlice map[int64]*timeSliceTelemetry

type timeSliceTelemetry struct {
	timeSlice   int64
	statusCodes EndpointStatusCodes
	latencies   ProxyEndpointLatenciesImpl
}

func newTimeSliceTelemetry(timeSlice int64) *timeSliceTelemetry {
	return &timeSliceTelemetry{
		timeSlice:   timeSlice,
		statusCodes: newEndpointStatusCodes(),
		latencies:   newProxyEndpointLatenciesImpl(), // TODO(mredolatti): in the future, check why this is not returning a pointer
	}
}

func keyForTimeSlice(t time.Time, intervalWidthInSeconds int64) int64 {
	curr := t.Unix()
	return curr - (curr % intervalWidthInSeconds)
}

// TimeSliceData splits the latest metrics in N entries of fixed x-seconds width timeslices
type TimeSliceData []ForTimeSlice

// ForTimeSlice stores all the data for a certain time-slice
type ForTimeSlice struct {
	TimeSlice int64                  `json:"timeslice"`
	Resources map[string]ForResource `json:"resources"`
}

// ForResource bundles latencies & status code for a specific timeslice
type ForResource struct {
	Latencies    []int64       `json:"latencies"`
	StatusCodes  map[int]int64 `json:"statusCodes"`
	RequestCount int           `json:"requestCount"`
}

func newForResource(latencies []int64, statusCodes map[int]int64) ForResource {
	var count int64
	for _, partialCount := range statusCodes {
		count += partialCount
	}

	return ForResource{
		Latencies:    latencies,
		StatusCodes:  statusCodes,
		RequestCount: int(count),
	}
}

func formatTimeSeriesData(data []*timeSliceTelemetry) TimeSliceData {
	sort.Slice(data, func(i, j int) bool { return data[i].timeSlice < data[j].timeSlice })
	toRet := make(TimeSliceData, 0, len(data))
	for _, ts := range data {
		toRet = append(toRet, ForTimeSlice{
			TimeSlice: ts.timeSlice,
			Resources: map[string]ForResource{
				"auth":                   newForResource(ts.latencies.auth.ReadAll(), ts.statusCodes.auth.peek()),
				"splitChanges":           newForResource(ts.latencies.splitChanges.ReadAll(), ts.statusCodes.splitChanges.peek()),
				"segmentChanges":         newForResource(ts.latencies.segmentChanges.ReadAll(), ts.statusCodes.segmentChanges.peek()),
				"mySegments":             newForResource(ts.latencies.mySegments.ReadAll(), ts.statusCodes.mySegments.peek()),
				"impressionsBulk":        newForResource(ts.latencies.impressionsBulk.ReadAll(), ts.statusCodes.impressionsBulk.peek()),
				"impressionsBulkBeacon":  newForResource(ts.latencies.impressionsBulkBeacon.ReadAll(), ts.statusCodes.impressionsBulkBeacon.peek()),
				"impressionsCount":       newForResource(ts.latencies.impressionsCount.ReadAll(), ts.statusCodes.impressionsCount.peek()),
				"impressionsCountBeacon": newForResource(ts.latencies.impressionsCountBeacon.ReadAll(), ts.statusCodes.impressionsCountBeacon.peek()),
				"eventsBulk":             newForResource(ts.latencies.eventsBulk.ReadAll(), ts.statusCodes.eventsBulk.peek()),
				"eventsBulkBeacon":       newForResource(ts.latencies.eventsBulkBeacon.ReadAll(), ts.statusCodes.eventsBulkBeacon.peek()),
				"telemetryConfig":        newForResource(ts.latencies.telemetryConfig.ReadAll(), ts.statusCodes.telemetryConfig.peek()),
				"telemetryRuntime":       newForResource(ts.latencies.telemetryRuntime.ReadAll(), ts.statusCodes.telemetryRuntime.peek()),
			},
		})
	}
	return toRet
}

// clock interface for mocking
type clock interface {
	Now() time.Time
}

type sysClock struct{}

func (c *sysClock) Now() time.Time { return time.Now() }

var _ TimeslicedProxyEndpointTelemetry = (*TimeslicedProxyEndpointTelemetryImpl)(nil)
var _ ProxyTelemetryPeeker = (*TimeslicedProxyEndpointTelemetryImpl)(nil)
var _ storage.TelemetryPeeker = (*TimeslicedProxyEndpointTelemetryImpl)(nil)
