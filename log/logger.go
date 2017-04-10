// Package log implements a custom log with option to send logs
// to stdout, file, and slack channel
package log

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var (
	// Verbose level
	Verbose *log.Logger
	// Debug level
	Debug *log.Logger
	// Info level
	Info *log.Logger
	// Warning level
	Warning *log.Logger
	// Error level
	Error *log.Logger
)

// SlackWriter writes messages to Slack user or channel. Implements io.Writer interface
type SlackWriter struct {
	WebHookURL  string
	Channel     string
	RefreshRate int
	message     []byte
	lastSent    int64
}

// Write the message to slack webhook
func (w *SlackWriter) Write(p []byte) (n int, err error) {
	w.message = append(w.message, p...)
	currentTime := time.Now().Unix()
	gapTime := int(currentTime - w.lastSent)
	if gapTime >= w.RefreshRate {
		err := w.postMessage()
		if err != nil {
			fmt.Println("[Slack]", err.Error())
		}
		//Drop current message
		w.message = w.message[:0]
		//Reset last sent time
		w.lastSent = currentTime
	}
	return len(p), nil
}

func (w *SlackWriter) postMessage() (err error) {
	urlStr := w.WebHookURL
	var jsonStr = fmt.Sprintf(`{"channel": "%s", "username": "splitio-go-agent", "text": "%s", "icon_emoji": ":robot_face:"}`, w.Channel, w.message)
	req, _ := http.NewRequest("POST", urlStr, bytes.NewBuffer([]byte(jsonStr)))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error posting log message to Slack")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		// If message has been written successfully (http 200 OK)
		return nil
	}
	body, _ := ioutil.ReadAll(resp.Body)
	return fmt.Errorf("Error posting log message to Slack %s, with message %s", resp.Status, body)
}

// Initialize log module
func Initialize(verboseWriter io.Writer,
	debugWriter io.Writer,
	infoWriter io.Writer,
	warningWriter io.Writer,
	errorWriter io.Writer) {

	Verbose = log.New(verboseWriter,
		"SPLITIO-AGENT | VERBOSE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Debug = log.New(debugWriter,
		"SPLITIO-AGENT | DEBUG: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoWriter,
		"SPLITIO-AGENT | INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(warningWriter,
		"SPLITIO-AGENT | WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(errorWriter,
		"SPLITIO-AGENT | ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}
