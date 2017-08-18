package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/util"
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
		received = false
		var data ImpressionBulk
		body, _ := ioutil.ReadAll(r.Body)
		json.Unmarshal(body, &data)
		t.Error(data)
		//		if data.Data != "123" {
		//			t.Error("Recieved data does not match")
		//		}

	}))
	defer ts.Close()

	ils := util.ImpressionListenerSubmitter{Endpoint: ts.URL}
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
