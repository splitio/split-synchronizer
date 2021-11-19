package task

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
)

type mockWorker struct {
	fetchCall        func() ([]string, error)
	processCall      func(rawData [][]byte, sink chan<- interface{}) error
	buildRequestCall func(data interface{}) (*http.Request, func(), error)
}

func (m *mockWorker) Fetch() ([]string, error) {
	return m.fetchCall()
}

func (m *mockWorker) Process(rawData [][]byte, sink chan<- interface{}) error {
	return m.processCall(rawData, sink)
}

func (m *mockWorker) BuildRequest(data interface{}) (*http.Request, func(), error) {
	return m.buildRequestCall(data)
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

type taskMemoryPoolWrapper struct {
	wrapped    *taskMemoryPoolImpl
	rawBuffers int64
}

func newTaskMemoryPoolWraper(size int) *taskMemoryPoolWrapper {
	return &taskMemoryPoolWrapper{wrapped: newTaskMemoryPool(size)}
}

func (p *taskMemoryPoolWrapper) validate(t *testing.T) {
	t.Helper()
	if r := atomic.LoadInt64(&p.rawBuffers); r != 0 {
		t.Error("possible leak in raw buffer: ", r)
	}
}

func (p *taskMemoryPoolWrapper) getRawBuffer() rawBuffer {
	atomic.AddInt64(&p.rawBuffers, 1)
	return p.wrapped.getRawBuffer()
}

func (p *taskMemoryPoolWrapper) releaseRawBuffer(b rawBuffer) {
	atomic.AddInt64(&p.rawBuffers, -1)
	p.wrapped.releaseRawBuffer(b)
}

func TestPipelineTask(t *testing.T) {
	var fetchChalls int64
	var processCalls int64
	var postCalls int64
	var httpCalls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&httpCalls, 1)
		if httpCalls > 4 {
			t.Error("there hsould be 4 requests. Currently have: ", httpCalls)
		}
		which := r.Header.Get("which")
		switch which {
		case "m1", "m2", "m3", "m4":
		default:
			t.Error("unrecognized header: ", which)
		}

	}))
	defer server.Close()
	w := &mockWorker{
		fetchCall: func() ([]string, error) {
			atomic.AddInt64(&fetchChalls, 1)
			if fetchChalls > 1 {
				// only send once
				return nil, nil
			}
			toRet := make([]string, defaultProcessBatchSize/2)
			for idx := 0; idx < defaultProcessBatchSize/2; idx++ {
				toRet = append(toRet, randSeq(100))
			}
			return toRet, nil
		},
		processCall: func(rawData [][]byte, sink chan<- interface{}) error {
			atomic.AddInt64(&processCalls, 1)
			if processCalls > 1 {
				t.Error("process should onlyvbe called once")
			}
			if len(rawData) != defaultProcessBatchSize {
				t.Error("it should accumulate batches of `defaultProcessBatchSize`")
			}

			sink <- "message1"
			sink <- "message2"
			sink <- "message3"
			sink <- "message4"
			return nil
		},
		buildRequestCall: func(data interface{}) (*http.Request, func(), error) {
			atomic.AddInt64(&postCalls, 1)
			if postCalls > 4 {
				t.Error("build request should be called 4 times only")
			}

			r, _ := http.NewRequest("GET", server.URL, nil)
			switch data {
			case "message1":
				r.Header.Add("which", "m1")
			case "message2":
				r.Header.Add("which", "m2")
			case "message3":
				r.Header.Add("which", "m3")
			case "message4":
				r.Header.Add("which", "m4")
			default:
				t.Error("invalid data ", data)
			}

			return r, nil, nil
		},
	}
	poolWrapper := newTaskMemoryPoolWraper(1000)
	cfg := &Config{
		Worker: w,
		Logger: logging.NewLogger(nil),
	}
	task, err := NewPipelinedTask(cfg)
	if err != nil {
		t.Error("task init: ", err)
	}
	task.pool = poolWrapper
	task.Start()
	time.Sleep(1 * time.Second)
	task.Stop(true)

	if fetchChalls < 1 || fetchChalls > 2 {
		t.Error("fetch should be called at most 2 times. Got: ", fetchChalls)
	}

	if processCalls != 1 {
		t.Error("fetch should be called only once. Got: ", processCalls)
	}

	if postCalls != 4 {
		t.Error("fetch should be called 4 times . Got: ", postCalls)
	}

	if httpCalls != 4 {
		t.Error("fetch should be called 4 times . Got: ", httpCalls)
	}

	poolWrapper.validate(t)
}
