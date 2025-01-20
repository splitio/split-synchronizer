package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v6/dtos"
	serviceMocks "github.com/splitio/go-split-commons/v6/service/mocks"
	"github.com/splitio/go-toolkit/v5/logging"
	ilmock "github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener/mocks"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	pstorageMocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"
	taskMocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks/mocks"
	"github.com/stretchr/testify/assert"
)

func TestSplitChangesEndpoints(t *testing.T) {
	opts := makeOpts()
	var splitStorage pstorageMocks.ProxySplitStorageMock
	opts.ProxySplitStorage = &splitStorage
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	// Test that a request without auth fails and is not cached
	status, _, _ := get("splitChanges?since=-1", opts.Port, nil)
	assert.Equal(t, 401, status)

	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "split1", ImpressionsDisabled: true}}}, nil).
		Once()

	// Make a proper request
	status, body, headers := get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)

	changes := toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Make another request, check we get the same response and the call count isn't incremented (cache is working)
	// Make a proper request
	status, body, headers = get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)

	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.True(t, changes.Splits[0].ImpressionsDisabled)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Lets evict the key (simulating a change in splits and re-check)
	splitStorage.On("ChangesSince", int64(-1), []string(nil)).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 2, Splits: []dtos.SplitDTO{{Name: "split2"}}}, nil).
		Once()

	opts.Cache.EvictBySurrogate(caching.SplitSurrogate)

	_, body, headers = get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(2), changes.Till)
	assert.Equal(t, "split2", changes.Splits[0].Name)
	assert.False(t, changes.Splits[0].ImpressionsDisabled)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

}

func TestSplitChangesWithFlagsetsCaching(t *testing.T) {
	opts := makeOpts()
	var splitStorage pstorageMocks.ProxySplitStorageMock
	opts.ProxySplitStorage = &splitStorage
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	splitStorage.On("ChangesSince", int64(-1), []string{"set1", "set2"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "split1"}}}, nil).
		Once()

	// Make a proper request
	status, body, headers := get("splitChanges?since=-1&sets=set2,set1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)

	changes := toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Make another request, check we get the same response and the call count isn't incremented (cache is working)
	status, body, headers = get("splitChanges?since=-1&sets=set2,set1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)

	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Make another request, with different flagsets. storage should be hit again
	splitStorage.On("ChangesSince", int64(-1), []string{"set1", "set2", "set3"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "split1"}}}, nil).
		Once()

	status, body, headers = get("splitChanges?since=-1&sets=set2,set1,set3", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)

	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Flush the cache, reset expectations, and retry the requests to make sure mocks are called again
	opts.Cache.EvictBySurrogate(caching.SplitSurrogate)

	splitStorage.On("ChangesSince", int64(-1), []string{"set1", "set2"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "split1"}}}, nil).
		Once()

	splitStorage.On("ChangesSince", int64(-1), []string{"set1", "set2", "set3"}).
		Return(&dtos.SplitChangesDTO{Since: -1, Till: 1, Splits: []dtos.SplitDTO{{Name: "split1"}}}, nil).
		Once()

	status, body, headers = get("splitChanges?since=-1&sets=set2,set1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)
	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	status, body, headers = get("splitChanges?since=-1&sets=set2,set1,set3", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	assert.Equal(t, 200, status)
	changes = toSplitChanges(body)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "split1", changes.Splits[0].Name)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
}

func TestSegmentChangesAndMySegmentsEndpoints(t *testing.T) {

	var segmentStorage pstorageMocks.ProxySegmentStorageMock

	opts := makeOpts()
	opts.ProxySegmentStorage = &segmentStorage
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	// Test that a request without auth fails and is not cached
	status, _, _ := get("segmentChanges/segment1?since=-1", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	// Same for mySegments
	status, _, _ = get("mySegments/k1", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	// Set up a response and make a proper request for segmentChanges
	segmentStorage.On("ChangesSince", "segment1", int64(-1)).
		Return(&dtos.SegmentChangesDTO{Since: -1, Till: 1, Name: "segment1", Added: []string{"k1"}, Removed: nil}, nil).
		Once()

	status, body, headers := get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes := toSegmentChanges(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "segment1", changes.Name)
	assert.Equal(t, []string{"k1"}, changes.Added)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Same for mysegments
	segmentStorage.On("SegmentsFor", "k1").Return([]string{"segment1"}, nil).Once()
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments := toMySegments(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, []dtos.MySegmentDTO{{Name: "segment1"}}, segments)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Update the response, make another request and check we get the same response and the call count isn't incremented (cache is working)
	segmentStorage.On("ChangesSince", "segment1", int64(-1)).
		Return(&dtos.SegmentChangesDTO{Since: -1, Till: 2, Name: "segment1", Added: []string{"k2"}, Removed: nil}, nil).
		Once()

	status, body, headers = get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSegmentChanges(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(1), changes.Till)
	assert.Equal(t, "segment1", changes.Name)
	assert.Equal(t, []string{"k1"}, changes.Added)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Same for mysegments
	segmentStorage.On("SegmentsFor", "k1").Return([]string{}, nil).Once()
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments = toMySegments(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, []dtos.MySegmentDTO{{Name: "segment1"}}, segments)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Lets evict the key (simulating a change in segment1 and re-check)
	opts.Cache.EvictBySurrogate(caching.MakeSurrogateForSegmentChanges("segment1"))
	status, body, headers = get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSegmentChanges(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(2), changes.Till)
	assert.Equal(t, "segment1", changes.Name)
	assert.Equal(t, []string{"k2"}, changes.Added)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))

	// Same for mysegments
	entries := caching.MakeMySegmentsEntries("k1")
	opts.Cache.Evict(entries[0])
	opts.Cache.Evict(entries[1])
	segmentStorage.On("SegmentsFor", "k1").Return([]string{}, nil).Once()
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments = toMySegments(body)
	assert.Equal(t, 200, status)
	assert.Equal(t, []dtos.MySegmentDTO{}, segments)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
}

func TestMembershipEndpoint(t *testing.T) {
	var segmentStorage pstorageMocks.ProxySegmentStorageMock
	var lsStorage pstorageMocks.ProxyLargeSegmentStorageMock

	opts := makeOpts()
	opts.ProxySegmentStorage = &segmentStorage
	opts.ProxyLargeSegmentStorage = &lsStorage
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	// Test that a request without auth fails and is not cached
	status, _, _ := get("memberships/mauro", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	segmentStorage.On("SegmentsFor", "mauro").Return([]string{"segment1"}, nil).Once()
	lsStorage.On("LargeSegmentsForUser", "mauro").Return([]string{"largeSegment1", "largeSegment2"}).Once()

	status, body, headers := get("memberships/mauro", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	response := memberships(body)
	expected := dtos.MembershipsResponseDTO{
		MySegments: dtos.Memberships{
			Segments: []dtos.Segment{{Name: "segment1"}},
		},
		MyLargeSegments: dtos.Memberships{
			Segments: []dtos.Segment{{Name: "largeSegment1"}, {Name: "largeSegment2"}},
		},
	}
	assert.Equal(t, 200, status)
	assert.Equal(t, expected, response)
	assert.Equal(t, "application/json; charset=utf-8", headers.Get("Content-Type"))
}

func makeOpts() *Options {
	return &Options{
		Logger:              logging.NewLogger(nil),
		Port:                rand.Intn(2000) + 2000,
		APIKeys:             []string{"someApiKey"},
		ImpressionListener:  &ilmock.ImpressionBulkListenerMock{},
		DebugOn:             true,
		SplitFetcher:        &serviceMocks.MockSplitFetcher{},
		ProxySplitStorage:   &pstorageMocks.ProxySplitStorageMock{},
		ProxySegmentStorage: &pstorageMocks.ProxySegmentStorageMock{},
		ImpressionsSink:     &taskMocks.MockDeferredRecordingTask{},
		ImpressionCountSink: &taskMocks.MockDeferredRecordingTask{},
		EventsSink:          &taskMocks.MockDeferredRecordingTask{},
		TelemetryConfigSink: &taskMocks.MockDeferredRecordingTask{},
		TelemetryUsageSink:  &taskMocks.MockDeferredRecordingTask{},
		Telemetry:           storage.NewProxyTelemetryFacade(),
		Cache:               caching.MakeProxyCache(),
	}
}

func get(path string, port int, headers map[string]string) (status int, body []byte, rheaders http.Header) {
	client := http.Client{}
	request, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/api/%s", port, path), nil)
	if err != nil {
		panic(err.Error())
	}

	for header, value := range headers {
		request.Header.Add(header, value)
	}

	resp, err := client.Do(request)
	if err != nil {
		panic(err.Error())
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err.Error())
	}

	return resp.StatusCode, body, resp.Header
}

func toSplitChanges(body []byte) dtos.SplitChangesDTO {
	var c dtos.SplitChangesDTO
	err := json.Unmarshal(body, &c)
	if err != nil {
		panic(err.Error())
	}
	return c
}

func toSegmentChanges(body []byte) dtos.SegmentChangesDTO {
	var c dtos.SegmentChangesDTO
	err := json.Unmarshal(body, &c)
	if err != nil {
		panic(err.Error())
	}
	return c
}

func toMySegments(body []byte) []dtos.MySegmentDTO {
	var c map[string][]dtos.MySegmentDTO
	err := json.Unmarshal(body, &c)
	if err != nil {
		panic(err.Error())
	}
	return c["mySegments"]
}

func memberships(body []byte) dtos.MembershipsResponseDTO {
	var c dtos.MembershipsResponseDTO
	err := json.Unmarshal(body, &c)
	if err != nil {
		panic(err.Error())
	}
	return c
}
