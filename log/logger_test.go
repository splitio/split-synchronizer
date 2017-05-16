// Package log implements a custom log with option to send logs
// to stdout, file, and slack channel
package log

import (
	"io/ioutil"
	"testing"
)

func TestInitialize(t *testing.T) {
	var commonWriter = ioutil.Discard

	Initialize(commonWriter, commonWriter, commonWriter, commonWriter, commonWriter)
}

func TestSlackWriter(t *testing.T) {
	slackWriter := &SlackWriter{WebHookURL: "", Channel: "", RefreshRate: 30}
	slackWriter.Write([]byte("Some error message"))

}
