package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/recorder"
)

func before() {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
}

func TestQueueImpressionsToPostToListener(t *testing.T) {
	QueueImpressionsForListener(&ImpressionBulk{})
	select {
	case in := <-impressionListenerStream:
		if in == nil {
			t.Error("Nil impression bulk queued")
		}
	default:
		t.Error("No impression bulk queue")
	}
}

func TestTaskPostImpressionsToListener(t *testing.T) {
	var received int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&received, 1)
		var data recorder.ImpressionListenerPostBody
		body, _ := ioutil.ReadAll(r.Body)
		err := json.Unmarshal(body, &data)
		if err != nil {
			t.Error("Error unmarshaling impressions bulk message in mocked impression listener")
		}
		var impData string
		err = json.Unmarshal(data.Impressions, &impData)
		if impData != "123" {
			t.Error("Recieved data does not match")
			t.Error(impData)
		}
	}))
	defer ts.Close()

	ils := recorder.ImpressionListenerSubmitter{Endpoint: ts.URL}
	failedQueue := make(chan *ImpressionBulk, 1)
	go taskPostImpressionsToListener(ils, failedQueue)

	impressionListenerStream <- &ImpressionBulk{
		Data: json.RawMessage(`"123"`),
	}

	time.Sleep(time.Duration(5) * time.Second)

	if atomic.LoadInt64(&received) <= 0 {
		t.Error("Message not received")
	}
}
