package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/recorder"
)

func before() {
	stdoutWriter := os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
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
	received := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		var data recorder.ImpressionListenerPostBody
		body, _ := ioutil.ReadAll(r.Body)
		err := json.Unmarshal(body, &data)
		if err != nil {
			t.Error("Error unmarshaling impression bulk in mocked impression listener")
		}
		if string(data.Impressions) != "[\"123\"]" {
			t.Error("Recieved data does not match")
			t.Error(string(data.Impressions))
		}

	}))
	defer ts.Close()

	ils := recorder.ImpressionListenerSubmitter{Endpoint: ts.URL}
	failedQueue := make(chan *ImpressionBulk, 1)
	go taskPostImpressionsToListener(ils, failedQueue)

	impressionListenerStream <- &ImpressionBulk{
		Data: json.RawMessage("[\"123\"]"),
	}

	time.Sleep(time.Duration(5) * time.Second)

	if !received {
		t.Error("Message not received")
	}
}
