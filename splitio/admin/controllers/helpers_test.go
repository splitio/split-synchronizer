package controllers

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/admin/views/dashboard"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/storage"
	"github.com/splitio/go-split-commons/v10/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"

	proxyStorage "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/stretchr/testify/assert"
)

func TestBundleRBInfo(t *testing.T) {
	split := &mocks.SplitStorageMock{}
	split.On("RuleBasedSegmentNames").Return(set.NewSet("rb1", "rb2"), nil).Once()
	rb := &mocks.MockRuleBasedSegmentStorage{}
	rb.On("GetRuleBasedSegmentByName", "rb1").Return(&dtos.RuleBasedSegmentDTO{Name: "rb1", ChangeNumber: 1, Status: "ACTIVE", Excluded: dtos.ExcludedDTO{Keys: []string{"one"}}}, nil).Once()
	rb.On("GetRuleBasedSegmentByName", "rb2").Return(&dtos.RuleBasedSegmentDTO{Name: "rb2", ChangeNumber: 2, Status: "ARCHIVED"}, nil).Once()
	result := bundleRuleBasedInfo(split, rb)
	assert.Len(t, result, 2)
	assert.ElementsMatch(t, result, []dashboard.RuleBasedSegmentSummary{
		{Name: "rb1", ChangeNumber: 1, Active: true, ExcludedKeys: []string{"one"}, ExcludedSegments: []dashboard.ExcludedSegments{}, LastModified: "Thu Jan  1 00:00:00 UTC 1970"},
		{Name: "rb2", ChangeNumber: 2, Active: false, ExcludedKeys: []string{}, ExcludedSegments: []dashboard.ExcludedSegments{}, LastModified: "Thu Jan  1 00:00:00 UTC 1970"},
	})
	split.AssertExpectations(t)
	rb.AssertExpectations(t)
}

// fakeProxyMetrics satisfies storage.TelemetryRuntimeConsumer via the embedded nil interface
// (never called by getProxyRequestCount) and proxyStorage.ProxyTelemetryPeeker.
type fakeProxyMetrics struct {
	storage.TelemetryRuntimeConsumer
	status map[int]map[int]int64
}

func (f *fakeProxyMetrics) PeekEndpointStatus(resource int) map[int]int64 {
	return f.status[resource]
}

func (f *fakeProxyMetrics) PeekEndpointLatency(resource int) []int64 {
	return nil
}

func TestGetProxyRequestCountAccumulatesErrors(t *testing.T) {
	m := &fakeProxyMetrics{status: map[int]map[int]int64{
		proxyStorage.SplitChangesEndpoint: {200: 5, 500: 2, 404: 3},
	}}
	ok, errored := getProxyRequestCount(m)
	assert.Equal(t, int64(5), ok)
	assert.Equal(t, int64(5), errored)
}

func TestGetProxyRequestCountAccumulatesErrorsAcrossEndpoints(t *testing.T) {
	m := &fakeProxyMetrics{status: map[int]map[int]int64{
		proxyStorage.SplitChangesEndpoint: {500: 2},
		proxyStorage.EventsBulkEndpoint:   {400: 4, 200: 1},
	}}
	ok, errored := getProxyRequestCount(m)
	assert.Equal(t, int64(1), ok)
	assert.Equal(t, int64(6), errored)
}

func TestGetProxyRequestCountCountsImpressionsCountBeacon(t *testing.T) {
	m := &fakeProxyMetrics{status: map[int]map[int]int64{
		proxyStorage.ImpressionsCountBeaconEndpoint: {200: 3},
	}}
	ok, errored := getProxyRequestCount(m)
	assert.Equal(t, int64(3), ok)
	assert.Equal(t, int64(0), errored)
}

func TestGetProxyRequestCountExcludesInternalEndpoints(t *testing.T) {
	m := &fakeProxyMetrics{status: map[int]map[int]int64{
		proxyStorage.TelemetryConfigEndpoint: {200: 9},
	}}
	ok, errored := getProxyRequestCount(m)
	assert.Equal(t, int64(0), ok)
	assert.Equal(t, int64(0), errored)
}

func TestGetProxyRequestCountNotProxyMode(t *testing.T) {
	ok, errored := getProxyRequestCount(&mocks.MockTelemetryStorage{})
	assert.Equal(t, int64(0), ok)
	assert.Equal(t, int64(0), errored)
}

func TestProxyCountedEndpointsHasNoDuplicates(t *testing.T) {
	seen := map[int]bool{}
	for _, res := range proxyCountedEndpoints {
		if seen[res] {
			t.Fatalf("endpoint %d is listed more than once", res)
		}
		seen[res] = true
	}
	assert.True(t, seen[proxyStorage.ImpressionsCountBeaconEndpoint], "ImpressionsCountBeaconEndpoint must be counted")
}
