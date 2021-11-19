package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	serviceMocks "github.com/splitio/go-split-commons/v4/service/mocks"
	"github.com/splitio/go-toolkit/v5/logging"
	ilmock "github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener/mocks"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/caching"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	pstorageMocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"
	taskMocks "github.com/splitio/split-synchronizer/v5/splitio/proxy/tasks/mocks"
)

func TestSplitChangesEndpoints(t *testing.T) {
	opts := makeOpts()
	var changesSinceCalls int64 = 0
	opts.ProxySplitStorage = &pstorageMocks.ProxySplitStorageMock{
		ChangesSinceCall: func(since int64) (*dtos.SplitChangesDTO, error) {
			atomic.AddInt64(&changesSinceCalls, 1)
			return &dtos.SplitChangesDTO{
				Since:  since,
				Till:   changesSinceCalls,
				Splits: []dtos.SplitDTO{{Name: fmt.Sprintf("split%d", changesSinceCalls)}},
			}, nil
		},
	}
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	// Test that a request without auth fails and is not cached
	status, _, _ := get("splitChanges?since=-1", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 0 {
		t.Error("auth middleware should have filtered this. expected 0 calls to handler. got: ", c)
	}

	// Make a proper request
	status, body, headers := get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes := toSplitChanges(body)
	if changes.Till != 1 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Splits[0].Name != "split1" {
		t.Error("wrong split name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Make another request, check we get the same response and the call count isn't incremented (cache is working)
	status, body, headers = get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSplitChanges(body)
	if changes.Till != 1 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Splits[0].Name != "split1" {
		t.Error("wrong split name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Lets evict the key (simulating a change in splits and re-check)
	opts.Cache.EvictBySurrogate(caching.SplitSurrogate)
	status, body, headers = get("splitChanges?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSplitChanges(body)
	if changes.Till != 2 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Splits[0].Name != "split2" {
		t.Error("wrong split name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 2 {
		t.Error("endpoint handler should have 2 call. has ", c)
	}
}

func TestSegmentChangesAndMySegmentsEndpoints(t *testing.T) {
	opts := makeOpts()
	var changesSinceCalls int64 = 0
	var mySegmentsCalls int64 = 0
	var changesToReturn atomic.Value
	var segmentsForToReturn atomic.Value
	opts.ProxySegmentStorage = &pstorageMocks.ProxySegmentStorageMock{
		ChangesSinceCall: func(name string, since int64) (*dtos.SegmentChangesDTO, error) {
			atomic.AddInt64(&changesSinceCalls, 1)
			return changesToReturn.Load().(*dtos.SegmentChangesDTO), nil
		},
		SegmentsForCall: func(key string) ([]string, error) {
			atomic.AddInt64(&mySegmentsCalls, 1)
			return segmentsForToReturn.Load().([]string), nil
		},
	}
	proxy := New(opts)
	go proxy.Start()
	time.Sleep(1 * time.Second) // Let the scheduler switch the current thread/gr and start the server

	// Test that a request without auth fails and is not cached
	status, _, _ := get("segmentChanges/segment1?since=-1", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 0 {
		t.Error("auth middleware should have filtered this. expected 0 calls to handler. got: ", c)
	}

	// Same for mySegments
	status, _, _ = get("mySegments/k1", opts.Port, nil)
	if status != 401 {
		t.Error("status should be 401. Is", status)
	}

	if c := atomic.LoadInt64(&mySegmentsCalls); c != 0 {
		t.Error("auth middleware should have filtered this. expected 0 calls to handler. got: ", c)
	}

	// Set up a response and make a proper request for segmentChanges
	changesToReturn.Store(&dtos.SegmentChangesDTO{Since: -1, Till: 1, Name: "segment1", Added: []string{"k1"}, Removed: nil})
	status, body, headers := get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes := toSegmentChanges(body)
	if changes.Till != 1 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Name != "segment1" {
		t.Error("wrong segment name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Same for mysegments
	segmentsForToReturn.Store([]string{"segment1"})
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments := toMySegments(body)
	if segments[0].Name != "segment1" {
		t.Error("wrong segment: ", segments[0])
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&mySegmentsCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Update the response, make another request and check we get the same response and the call count isn't incremented (cache is working)
	changesToReturn.Store(&dtos.SegmentChangesDTO{Since: -1, Till: 2, Name: "segment1", Added: []string{"k2"}, Removed: nil})
	status, body, headers = get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSegmentChanges(body)
	if changes.Till != 1 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Name != "segment1" {
		t.Error("wrong segment name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Same for mysegments
	segmentsForToReturn.Store([]string{})
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments = toMySegments(body)
	if segments[0].Name != "segment1" {
		t.Error("wrong segment: ", segments[0])
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&mySegmentsCalls); c != 1 {
		t.Error("endpoint handler should have 1 call. has ", c)
	}

	// Lets evict the key (simulating a change in segment1 and re-check)
	opts.Cache.EvictBySurrogate(caching.MakeSurrogateForSegmentChanges("segment1"))
	status, body, headers = get("segmentChanges/segment1?since=-1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	changes = toSegmentChanges(body)
	if changes.Till != 2 {
		t.Error("wrong till: ", changes.Till)
	}

	if changes.Name != "segment1" {
		t.Error("wrong segment name")
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&changesSinceCalls); c != 2 {
		t.Error("endpoint handler should have 2 call. has ", c)
	}

	// Same for mysegments
	opts.Cache.Evict(caching.MakeMySegmentsEntry("k1"))
	status, body, headers = get("mySegments/k1", opts.Port, map[string]string{"Authorization": "Bearer someApiKey"})
	segments = toMySegments(body)
	if len(segments) != 0 {
		t.Error("wrong segment: ", segments)
	}

	if ce := headers.Get("Content-Type"); ce != "application/json; charset=utf-8" {
		t.Error("wrong content type: ", ce)
	}

	if c := atomic.LoadInt64(&mySegmentsCalls); c != 2 {
		t.Error("endpoint handler should have 2 call. has ", c)
	}
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
		fmt.Println(string(body))
		panic(err.Error())
	}
	return c["mySegments"]
}
