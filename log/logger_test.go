// Package log implements a custom log with option to send logs
// to stdout, file, and slack channel
package log

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/go-toolkit/v3/logging"
)

func TestInitialize(t *testing.T) {
	var commonWriter = ioutil.Discard

	Initialize(commonWriter, commonWriter, commonWriter, commonWriter, commonWriter, logging.LevelNone)
}

func TestSlackWriter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedJSON := `{"channel": "some-channel", "username": "Split-Sync", "text": "Some error message", "icon_emoji": ":robot_face:"}`
		rBody, _ := ioutil.ReadAll(r.Body)

		if string(rBody) != expectedJSON {
			t.Error("malformed JSON at SLACK message")
		}

		fmt.Fprintln(w, "ok")
	}))
	defer ts.Close()

	slackWriter := &SlackWriter{WebHookURL: ts.URL, Channel: "some-channel", RefreshRate: 30}
	slackWriter.Write([]byte("Some error message"))
}
