package storage

import (
	"sort"
	"sync"
	"time"
)

// Granularity selection constants to be used upon component instantiation
const (
	HistoricTelemetryGranularityMinute = iota
	HistoricTelemetryGranularityHour
	HistoricTelemetryGranularityDay
)

type telemetryByTimeSlice map[int64]*timeSliceTelemetry

// TimeslicedProxyEndpointTelemetry is a proxy telemetry facade (yet another) that bundles global data
// and historic data by timeslice (for observability purposes)
type TimeslicedProxyEndpointTelemetry interface {
	ProxyEndpointTelemetry
	TimeslicedReport() TimeSliceData
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

	// format & return
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
type TimeSliceData []map[string]ForResource

// ForResource bundles latencies & status code for a specific timeslice
type ForResource struct {
	TimeSlice   int64         `json:"timeslice"`
	Latencies   []int64       `json:"latencies"`
	StatusCodes map[int]int64 `json:"statusCodes"`
}

func formatTimeSeriesData(data []*timeSliceTelemetry) TimeSliceData {
	sort.Slice(data, func(i, j int) bool { return data[i].timeSlice < data[j].timeSlice })
	toRet := make(TimeSliceData, 0, len(data))
	for _, ts := range data {
		forTS := map[string]ForResource{
			"auth": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.auth.ReadAll(),
				StatusCodes: ts.statusCodes.auth.peek(),
			},
			"splitChanges": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.splitChanges.ReadAll(),
				StatusCodes: ts.statusCodes.splitChanges.peek(),
			},
			"segmentChanges": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.segmentChanges.ReadAll(),
				StatusCodes: ts.statusCodes.segmentChanges.peek(),
			},
			"impressionsBulk": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.impressionsBulk.ReadAll(),
				StatusCodes: ts.statusCodes.impressionsBulk.peek(),
			},
			"impressionsBulkBeacon": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.impressionsBulkBeacon.ReadAll(),
				StatusCodes: ts.statusCodes.impressionsBulkBeacon.peek(),
			},
			"impressionsCount": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.impressionsCount.ReadAll(),
				StatusCodes: ts.statusCodes.impressionsCount.peek(),
			},
			"impressionsCountBeacon": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.impressionsCountBeacon.ReadAll(),
				StatusCodes: ts.statusCodes.impressionsCountBeacon.peek(),
			},
			"eventsBulk": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.eventsBulk.ReadAll(),
				StatusCodes: ts.statusCodes.eventsBulk.peek(),
			},
			"eventsBulkBeacon": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.eventsBulkBeacon.ReadAll(),
				StatusCodes: ts.statusCodes.eventsBulkBeacon.peek(),
			},
			"telemetryConfig": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.telemetryConfig.ReadAll(),
				StatusCodes: ts.statusCodes.telemetryConfig.peek(),
			},
			"telemetryRuntime": ForResource{
				TimeSlice:   ts.timeSlice,
				Latencies:   ts.latencies.telemetryRuntime.ReadAll(),
				StatusCodes: ts.statusCodes.telemetryRuntime.peek(),
			},
		}
		toRet = append(toRet, forTS)
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
