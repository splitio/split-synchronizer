package task

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/storage/mocks"
	"github.com/splitio/go-toolkit/v5/logging"
)

type eventTrackingAllocator struct {
	pool           *eventsMemoryPoolImpl
	metadataMaps   int64
	events         int64
	eventsWithMeta int64
}

func (p *eventTrackingAllocator) validate(t *testing.T) {
	t.Helper()
	if r := atomic.LoadInt64(&p.metadataMaps); r != 0 {
		t.Error("possible leak in metadata maps: ", r)
	}

	if r := atomic.LoadInt64(&p.events); r != 0 {
		t.Error("possible leak in event slices: ", r)
	}

	if r := atomic.LoadInt64(&p.eventsWithMeta); r != 0 {
		t.Error("possible leak in eventswithMeta: ", r)
	}
}

func (p *eventTrackingAllocator) acquireEvents() eventsSlice {
	atomic.AddInt64(&p.events, 1)
	return p.pool.acquireEvents()
}

func (p *eventTrackingAllocator) releaseEvents(e eventsSlice) {
	atomic.AddInt64(&p.events, -1)
	p.pool.releaseEvents(e)
}

func (p *eventTrackingAllocator) acquireMetadataMap() metadataMap {
	atomic.AddInt64(&p.metadataMaps, 1)
	return p.pool.acquireMetadataMap()
}

func (p *eventTrackingAllocator) releaseMetadataMap(m metadataMap) {
	atomic.AddInt64(&p.metadataMaps, -1)
	p.pool.releaseMetadataMap(m)
}

func (p *eventTrackingAllocator) acquireEventsWithMeta() eventsWithMetaSlice {
	atomic.AddInt64(&p.eventsWithMeta, 1)
	return p.pool.acquireEventsWithMeta()
}

func (p *eventTrackingAllocator) releaseEventsWithMeta(t eventsWithMetaSlice) {
	atomic.AddInt64(&p.eventsWithMeta, -1)
	p.pool.releaseEventsWithMeta(t)
}

func newEventTrackingAllocator() *eventTrackingAllocator {
	return &eventTrackingAllocator{pool: newEventWorkerMemoryPool(10000, defaultMetasPerBulk, defaultEventsPerBulk)}
}

func makeSerializedEvents(metadatas int, keys int) [][]byte {
	result := func(r []byte, _ error) []byte { return r }
	evs := make([][]byte, 0, metadatas*keys)
	for mindex := 0; mindex < metadatas; mindex++ {
		metadata := dtos.Metadata{SDKVersion: "go-1.1.1", MachineName: "machine_" + strconv.Itoa(mindex)}
		for eindex := 0; eindex < keys; eindex++ {
			evs = append(evs, result(json.Marshal(&dtos.QueueStoredEventDTO{
				Metadata: metadata,
				Event:    dtos.EventDTO{Key: "key_" + strconv.Itoa(eindex), Timestamp: int64(1 + mindex*eindex)},
			})))
		}
	}
	return evs
}

func TestEventsMemoryIsProperlyReturned(t *testing.T) {
	poolWrapper := newEventTrackingAllocator()
	w, err := NewEventsWorker(&EventWorkerConfig{
		Logger:    logging.NewLogger(nil),
		Storage:   mocks.MockEventStorage{},
		URL:       "http://test",
		Apikey:    "someApikey",
		FetchSize: 100,
	})
	w.pool = poolWrapper
	if err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	sinker := make(chan interface{}, 100)
	w.Process(makeSerializedEvents(3, 100), sinker)
	if len(sinker) != 3 {
		t.Error("there should be 3 bulks ready for submission")
	}

	for i := 0; i < 3; i++ {
		req, cb, err := w.BuildRequest(<-sinker)
		cb()
		if req == nil || err != nil {
			t.Error("there should be no error. Got: ", err)
		}
	}
	poolWrapper.validate(t)
}

func TestEventsIntegration(t *testing.T) {
	var mtx sync.Mutex
	evsByMachineName := make(map[string]int, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error("error reading body")
		}

		var ti []dtos.EventDTO
		if err := json.Unmarshal(body, &ti); err != nil {
			fmt.Println(string(body))
			t.Error("error deserializing body: ", err)
		}

		machine := r.Header.Get("SplitSDKMachineName")
		mtx.Lock()
		evsByMachineName[machine] = evsByMachineName[machine] + len(ti)
		mtx.Unlock()

	}))
	defer server.Close()

	evs := makeSerializedEvents(3, 100)
	var calls int64
	st := &mocks.MockEventStorage{
		PopNRawCall: func(int64) ([]string, error) {
			atomic.AddInt64(&calls, 1)
			if atomic.LoadInt64(&calls) > 500 {
				return nil, nil
			}
			asStr := make([]string, 0, len(evs))
			for idx := range evs {
				asStr = append(asStr, string(evs[idx]))
			}
			return asStr, nil
		},
	}

	poolWrapper := newEventTrackingAllocator()
	w, err := NewEventsWorker(&EventWorkerConfig{
		Logger:    logging.NewLogger(nil),
		Storage:   st,
		URL:       server.URL,
		Apikey:    "someApikey",
		FetchSize: 5000,
	})
	if err != nil {
		t.Error("worker instantiation should not fail: ", err)
	}
	w.pool = poolWrapper

	task, err := NewPipelinedTask(&Config{
		Logger:       logging.NewLogger(nil),
		Worker:       w,
		MaxAccumWait: 500 * time.Millisecond,
	})
	if err != nil {
		t.Error("task instantiation should not fail: ", err)
	}

	task.Start()
	time.Sleep(2 * time.Second)
	task.Stop(true)

	if l := len(evsByMachineName); l != 3 {
		t.Error("there should be 3 different metas. there are: ", l)
	}

	expectedEventsPerMeta := 500 * 100 // bulks * events
	if r := evsByMachineName["machine_0"]; r != expectedEventsPerMeta {
		t.Error("machine0 should have 500 events. Has ", r)
	}
	if r := evsByMachineName["machine_1"]; r != expectedEventsPerMeta {
		t.Error("machine1 should have 500 events. Has ", r)
	}
	if r := evsByMachineName["machine_2"]; r != expectedEventsPerMeta {
		t.Error("machine2 should have 500 events. Has ", r)
	}
}
