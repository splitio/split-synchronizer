package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/provisional"
	"github.com/splitio/go-split-commons/v4/provisional/strategy"
	"github.com/splitio/go-split-commons/v4/storage/inmemory"
	"github.com/splitio/go-split-commons/v4/storage/mocks"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"
)

type trackingAllocator struct {
	pool            *impressionsMemoryPoolImpl
	keyImpSlices    int64
	metadataMaps    int64
	featureNameMaps int64
	testImpressions int64
	impsWithMeta    int64
}

func (p *trackingAllocator) validate(t *testing.T) {
	t.Helper()
	if r := atomic.LoadInt64(&p.keyImpSlices); r != 0 {
		t.Error("possible leak in key impression slices: ", r)
	}

	if r := atomic.LoadInt64(&p.metadataMaps); r != 0 {
		t.Error("possible leak in metadata maps: ", r)
	}

	if r := atomic.LoadInt64(&p.featureNameMaps); r != 0 {
		t.Error("possible leak in feature name maps: ", r)
	}

	if r := atomic.LoadInt64(&p.testImpressions); r != 0 {
		t.Error("possible leak in test impression slices: ", r)
	}

	if r := atomic.LoadInt64(&p.impsWithMeta); r != 0 {
		t.Error("possible leak in impressionWithMeta bundles: ", r)
	}
}

func (p *trackingAllocator) acquireKeyImpressions() keyImpressionsSlice {
	atomic.AddInt64(&p.keyImpSlices, 1)
	return p.pool.acquireKeyImpressions()
}

func (p *trackingAllocator) releaseKeyImpressions(k keyImpressionsSlice) {
	atomic.AddInt64(&p.keyImpSlices, -1)
	p.pool.releaseKeyImpressions(k)
}

func (p *trackingAllocator) acquireMetadataMap() metadataMap {
	atomic.AddInt64(&p.metadataMaps, 1)
	return p.pool.acquireMetadataMap()
}

func (p *trackingAllocator) releaseMetadataMap(m metadataMap) {
	atomic.AddInt64(&p.metadataMaps, -1)
	p.pool.releaseMetadataMap(m)
}

func (p *trackingAllocator) acquireFeatureNameMap() featureNameMap {
	atomic.AddInt64(&p.featureNameMaps, 1)
	return p.pool.acquireFeatureNameMap()
}

func (p *trackingAllocator) releaseFeatureNameMap(m featureNameMap) {
	atomic.AddInt64(&p.featureNameMaps, -1)
	p.pool.releaseFeatureNameMap(m)
}

func (p *trackingAllocator) acquireTestImpressions() testImpressionsSlice {
	atomic.AddInt64(&p.testImpressions, 1)
	return p.pool.acquireTestImpressions()
}

func (p *trackingAllocator) releaseTestImpressions(t testImpressionsSlice) {
	atomic.AddInt64(&p.testImpressions, -1)
	p.pool.releaseTestImpressions(t)
}

func (p *trackingAllocator) acquireImpsWithMeta() impsWithMetaSlice {
	atomic.AddInt64(&p.impsWithMeta, 1)
	return p.pool.acquireImpsWithMeta()
}

func (p *trackingAllocator) releaseImpsWithMeta(t impsWithMetaSlice) {
	atomic.AddInt64(&p.impsWithMeta, -1)
	p.pool.releaseImpsWithMeta(t)
}

func newTrackingAllocator() *trackingAllocator {
	return &trackingAllocator{pool: newImpWorkerMemoryPool(10000, defaultMetasPerBulk, defaultFeatureCount, defaultImpsPerFeature)}
}

func makeSerializedImpressions(metadatas int, features int, keys int) [][]byte {
	result := func(r []byte, _ error) []byte { return r }
	imps := make([][]byte, 0, metadatas*features*keys)
	for mindex := 0; mindex < metadatas; mindex++ {
		metadata := dtos.Metadata{SDKVersion: "go-1.1.1", MachineName: "machine_" + strconv.Itoa(mindex)}
		for findex := 0; findex < features; findex++ {
			feature := "feat_" + strconv.Itoa(findex)
			for kindex := 0; kindex < keys; kindex++ {
				imps = append(imps, result(json.Marshal(&dtos.ImpressionQueueObject{
					Metadata:   metadata,
					Impression: dtos.Impression{FeatureName: feature, KeyName: "key_" + strconv.Itoa(kindex), Time: int64(1 + mindex*findex*kindex)},
				})))
			}
		}
	}
	return imps
}

func TestMemoryIsProperlyReturned(t *testing.T) {
	impressionsCounter := strategy.NewImpressionsCounter()
	impressionObserver, _ := strategy.NewImpressionObserver(500)
	strategy := strategy.NewOptimizedImpl(impressionObserver, impressionsCounter, &inmemory.TelemetryStorage{}, false)

	poolWrapper := newTrackingAllocator()
	w, err := NewImpressionWorker(&ImpressionWorkerConfig{
		EvictionMonitor:     evcalc.New(1),
		Logger:              logging.NewLogger(nil),
		ImpressionsListener: nil,
		Storage:             mocks.MockImpressionStorage{},
		URL:                 "http://test",
		Apikey:              "someApikey",
		FetchSize:           100,
		ImpressionManager:   provisional.NewImpressionManager(strategy),
	})
	w.pool = poolWrapper
	if err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	sinker := make(chan interface{}, 100)
	w.Process(makeSerializedImpressions(3, 4, 20), sinker)
	if len(sinker) != 3 {
		t.Error("there should be 3 bulks ready for submission")
	}

	for i := 0; i < 3; i++ {
		i := <- sinker
		req, err := w.BuildRequest(i)
		if asRecyclable, ok := i.(recyclable); ok {
			asRecyclable.recycle()
		}

		if req == nil || err != nil {
			t.Error("there should be no error. Got: ", err)
		}
	}
	poolWrapper.validate(t)
}

func TestImpressionsIntegration(t *testing.T) {

	var mtx sync.Mutex
	impsByMachineName := make(map[string]int, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error("error reading body")
		}

		var ti []dtos.ImpressionsDTO
		if err := json.Unmarshal(body, &ti); err != nil {
			t.Error("error deserializing body: ", err)
		}

		impressionsCount := 0
		for idx := range ti {
			impressionsCount += len(ti[idx].KeyImpressions)
		}

		machine := r.Header.Get("SplitSDKMachineName")
		mtx.Lock()
		impsByMachineName[machine] = impsByMachineName[machine] + impressionsCount
		mtx.Unlock()

	}))
	defer server.Close()

	imps := makeSerializedImpressions(3, 4, 20)
	var calls int64
	st := &mocks.MockImpressionStorage{
		PopNRawCall: func(int64) ([]string, int64, error) {
			atomic.AddInt64(&calls, 1)
			if atomic.LoadInt64(&calls) > 500 {
				return nil, 0, nil
			}
			asStr := make([]string, 0, len(imps))
			for idx := range imps {
				asStr = append(asStr, string(imps[idx]))
			}
			return asStr, 500000, nil
		},
	}

	impressionsCounter := strategy.NewImpressionsCounter()
	impressionObserver, _ := strategy.NewImpressionObserver(500)
	strategy := strategy.NewOptimizedImpl(impressionObserver, impressionsCounter, &inmemory.TelemetryStorage{}, false)

	poolWrapper := newTrackingAllocator()
	w, err := NewImpressionWorker(&ImpressionWorkerConfig{
		EvictionMonitor:     evcalc.New(1),
		Logger:              logging.NewLogger(&logging.LoggerOptions{LogLevel: logging.LevelDebug}),
		ImpressionsListener: nil,
		Storage:             st,
		URL:                 server.URL,
		Apikey:              "someApikey",
		FetchSize:           5000,
		ImpressionManager:   provisional.NewImpressionManager(strategy),
	})
	if err != nil {
		t.Error("worker instantiation should not fail: ", err)
	}
	w.pool = poolWrapper

	task, err := NewPipelinedTask(&Config{
		Logger:       logging.NewLogger(&logging.LoggerOptions{LogLevel: logging.LevelError}),
		Worker:       w,
		MaxAccumWait: 500 * time.Millisecond,
	})
	if err != nil {
		t.Error("task instantiation should not fail: ", err)
	}

	task.Start()
	time.Sleep(100 * time.Millisecond)
	task.Stop(true)

	if l := len(impsByMachineName); l != 3 {
		t.Error("there should be 3 different metas. there are: ", l)
	}

	expectedImpressionsPerMeta := 500 * 4 * 20 // bulks * features * keys
	if r := impsByMachineName["machine_0"]; r != expectedImpressionsPerMeta {
		t.Errorf("machine0 should have %d impressions. Has %d", expectedImpressionsPerMeta, r)
	}
	if r := impsByMachineName["machine_1"]; r != expectedImpressionsPerMeta {
		t.Errorf("machine0 should have %d impressions. Has %d", expectedImpressionsPerMeta, r)
	}
	if r := impsByMachineName["machine_2"]; r != expectedImpressionsPerMeta {
		t.Errorf("machine0 should have %d impressions. Has %d", expectedImpressionsPerMeta, r)
	}
}

