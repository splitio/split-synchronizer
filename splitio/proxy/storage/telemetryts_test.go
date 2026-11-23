package storage

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type mockClock struct {
	base  time.Time
	count int
}

func (c *mockClock) Now() time.Time {
	c.count++
	return c.base.Add(time.Duration(c.count) * time.Millisecond)
}

func TestHistoricProxyTelemetry(t *testing.T) {
	clk := mockClock{base: time.Now()}
	toWrap := NewProxyTelemetryFacade()
	timesliced := NewTimeslicedProxyEndpointTelemetry(toWrap, 60, 5)
	timesliced.clock = &clk

	endpoints := []int{
		AuthEndpoint,
		SplitChangesEndpoint,
		SegmentChangesEndpoint,
		MySegmentsEndpoint,
		ImpressionsBulkEndpoint,
		ImpressionsBulkBeaconEndpoint,
		ImpressionsCountEndpoint,
		ImpressionsCountBeaconEndpoint,
		EventsBulkEndpoint,
		EventsBulkBeaconEndpoint,
		TelemetryConfigEndpoint,
		TelemetryRuntimeEndpoint,
		TelemetryRuntimeBeaconEndpoint,
		TelemetryKeysClientSideEndpoint,
		TelemetryKeysClientSideBeaconEndpoint,
		TelemetryKeysServerSideEndpoint,
	}

	oldestTs := keyForTimeSlice(clk.base, 60) // store the oldest timeslice, so we can see it's no longet present after eviction
	for idx := 0; idx < 5; idx++ {
		// add metrics to endpoints
		for _, endpoint := range endpoints {
			timesliced.RecordEndpointLatency(endpoint, 1*time.Nanosecond) // put a one on the FIRST slot of each latency bucket array
			timesliced.RecordEndpointLatency(endpoint, 5*time.Hour)       // put a one on the LAST slot of each latency bucket array
			timesliced.IncrEndpointStatus(endpoint, 200)                  // add a successful call
			timesliced.IncrEndpointStatus(endpoint, 500)                  // add a failed call
		}
		clk.base = clk.base.Add(60 * time.Second)
	}

	if len(timesliced.telemetryByTimeSlice) != 5 {
		t.Error("there should be records in 5 timeslices")
	}

	if _, ok := timesliced.telemetryByTimeSlice[oldestTs]; !ok {
		t.Error("the oldest TS shuoldn't have been evicted yet")
	}

	// add a 6th timeslice (oldest should be cropped out)
	for _, endpoint := range endpoints {
		timesliced.RecordEndpointLatency(endpoint, 1*time.Nanosecond) // put a one on the FIRST slot of each latency bucket array
		timesliced.RecordEndpointLatency(endpoint, 1*time.Hour)       // put a one on the LAST slot of each latency bucket array
		timesliced.IncrEndpointStatus(endpoint, 200)                  // add a successful call
		timesliced.IncrEndpointStatus(endpoint, 500)                  // add a failed call
	}

	if len(timesliced.telemetryByTimeSlice) != 5 {
		t.Error("there should be records in 5 timeslices")
	}

	if _, ok := timesliced.telemetryByTimeSlice[oldestTs]; ok {
		t.Error("the oldest TS shuoldn have been evicted after adding the 6th timeslice")
	}

	// manually build the expected report and check it against the generated one
	expectedStatusCodes := map[int]int64{200: 1, 500: 1}
	expectedLatencies := []int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	expectedData := TimeSliceData{}
	for _, ts := range []int64{oldestTs + 60, oldestTs + 120, oldestTs + 180, oldestTs + 240, oldestTs + 300} {
		expectedData = append(expectedData, ForTimeSlice{
			TimeSlice: ts,
			Resources: map[string]ForResource{
				"auth":                          {expectedLatencies, expectedStatusCodes, 2},
				"splitChanges":                  {expectedLatencies, expectedStatusCodes, 2},
				"segmentChanges":                {expectedLatencies, expectedStatusCodes, 2},
				"mySegments":                    {expectedLatencies, expectedStatusCodes, 2},
				"impressionsBulk":               {expectedLatencies, expectedStatusCodes, 2},
				"impressionsBulkBeacon":         {expectedLatencies, expectedStatusCodes, 2},
				"impressionsCount":              {expectedLatencies, expectedStatusCodes, 2},
				"impressionsCountBeacon":        {expectedLatencies, expectedStatusCodes, 2},
				"eventsBulk":                    {expectedLatencies, expectedStatusCodes, 2},
				"eventsBulkBeacon":              {expectedLatencies, expectedStatusCodes, 2},
				"telemetryConfig":               {expectedLatencies, expectedStatusCodes, 2},
				"telemetryRuntime":              {expectedLatencies, expectedStatusCodes, 2},
				"telemetryBeaconRuntime":        {expectedLatencies, expectedStatusCodes, 2},
				"telemetryKeysClientSide":       {expectedLatencies, expectedStatusCodes, 2},
				"telemetryKeysClientSideBeacon": {expectedLatencies, expectedStatusCodes, 2},
				"telemetryKeysServerSide":       {expectedLatencies, expectedStatusCodes, 2},
			},
		})
	}

	generated := timesliced.TimeslicedReport()
	if lgen, lexp := len(generated), len(expectedData); lgen != lexp {
		t.Error("generated & expected have different lengths: ", lgen, lexp)
		return // to avoid panicking on the test below
	}

	for idx := range generated {
		if !reflect.DeepEqual(generated[idx], expectedData[idx]) {
			t.Errorf("index: %d - generated & expected data don't match", idx)
			t.Errorf("generated: %+v", generated[idx])
			t.Errorf("expected: %+v", expectedData[idx])
		}
	}

	// we update latencies & status codes with global number (everything was called 6 times)
	expectedStatusCodes = map[int]int64{200: 6, 500: 6}
	expectedLatencies = []int64{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6}
	expectedTotalReport := map[string]ForResource{
		"auth":                          {expectedLatencies, expectedStatusCodes, 12},
		"splitChanges":                  {expectedLatencies, expectedStatusCodes, 12},
		"segmentChanges":                {expectedLatencies, expectedStatusCodes, 12},
		"mySegments":                    {expectedLatencies, expectedStatusCodes, 12},
		"impressionsBulk":               {expectedLatencies, expectedStatusCodes, 12},
		"impressionsBulkBeacon":         {expectedLatencies, expectedStatusCodes, 12},
		"impressionsCount":              {expectedLatencies, expectedStatusCodes, 12},
		"impressionsCountBeacon":        {expectedLatencies, expectedStatusCodes, 12},
		"eventsBulk":                    {expectedLatencies, expectedStatusCodes, 12},
		"eventsBulkBeacon":              {expectedLatencies, expectedStatusCodes, 12},
		"telemetryConfig":               {expectedLatencies, expectedStatusCodes, 12},
		"telemetryRuntime":              {expectedLatencies, expectedStatusCodes, 12},
		"telemetryBeaconRuntime":        {expectedLatencies, expectedStatusCodes, 12},
		"telemetryKeysClientSide":       {expectedLatencies, expectedStatusCodes, 12},
		"telemetryKeysClientSideBeacon": {expectedLatencies, expectedStatusCodes, 12},
		"telemetryKeysServerSide":       {expectedLatencies, expectedStatusCodes, 12},
	}

	if gen := timesliced.TotalMetricsReport(); !reflect.DeepEqual(expectedTotalReport, gen) {
		t.Error("generated total report differs frome expected one")
		jsonGen, _ := json.Marshal(gen)
		jsonExp, _ := json.Marshal(expectedTotalReport)
		t.Errorf("generated: %+v", string(jsonGen))
		t.Errorf("expected: %+v", string(jsonExp))
	}
}

func TestTelemetryEndpointsAreIsolated(t *testing.T) {
	clk := mockClock{base: time.Now()}
	timesliced := NewTimeslicedProxyEndpointTelemetry(NewProxyTelemetryFacade(), 60, 5)
	timesliced.clock = &clk

	// distinct latency buckets for config & runtime
	timesliced.RecordEndpointLatency(TelemetryConfigEndpoint, 1*time.Nanosecond)
	timesliced.RecordEndpointLatency(TelemetryRuntimeEndpoint, 5*time.Hour)

	// distinct status codes (and request counts) for runtime & each keys endpoint
	timesliced.IncrEndpointStatus(TelemetryRuntimeEndpoint, 200)
	for i := 0; i < 2; i++ {
		timesliced.IncrEndpointStatus(TelemetryKeysClientSideEndpoint, 201)
	}
	for i := 0; i < 3; i++ {
		timesliced.IncrEndpointStatus(TelemetryKeysClientSideBeaconEndpoint, 202)
	}
	for i := 0; i < 4; i++ {
		timesliced.IncrEndpointStatus(TelemetryKeysServerSideEndpoint, 500)
	}

	firstBucket := make([]int64, 23)
	firstBucket[0] = 1
	lastBucket := make([]int64, 23)
	lastBucket[22] = 1
	noLatencies := make([]int64, 23)

	// latencies are bucketed independently
	if got := timesliced.PeekEndpointLatency(TelemetryConfigEndpoint); !reflect.DeepEqual(firstBucket, got) {
		t.Errorf("config latencies: expected %v, got %v", firstBucket, got)
	}
	if got := timesliced.PeekEndpointLatency(TelemetryRuntimeEndpoint); !reflect.DeepEqual(lastBucket, got) {
		t.Errorf("runtime latencies: expected %v, got %v", lastBucket, got)
	}

	report := timesliced.TotalMetricsReport()
	assertResource := func(name string, expected ForResource) {
		t.Helper()
		if got := report[name]; !reflect.DeepEqual(expected, got) {
			t.Errorf("total report for %s: expected %+v, got %+v", name, expected, got)
		}
	}
	assertResource("telemetryConfig", ForResource{firstBucket, map[int]int64{}, 0})
	assertResource("telemetryRuntime", ForResource{lastBucket, map[int]int64{200: 1}, 1})
	assertResource("telemetryKeysClientSide", ForResource{noLatencies, map[int]int64{201: 2}, 2})
	assertResource("telemetryKeysClientSideBeacon", ForResource{noLatencies, map[int]int64{202: 3}, 3})
	assertResource("telemetryKeysServerSide", ForResource{noLatencies, map[int]int64{500: 4}, 4})

	sliced := timesliced.TimeslicedReport()
	if len(sliced) != 1 {
		t.Fatalf("expected exactly 1 timeslice, got %d", len(sliced))
	}
	for name, expected := range report {
		if got := sliced[0].Resources[name]; !reflect.DeepEqual(expected, got) {
			t.Errorf("timesliced report for %s: expected %+v, got %+v", name, expected, got)
		}
	}
}
