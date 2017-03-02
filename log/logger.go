package log

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
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
	WebHookURL string
	Channel    string
}

// Write the message to slack webhook
func (w *SlackWriter) Write(p []byte) (n int, err error) {
	urlStr := w.WebHookURL
	fmt.Println("URL:>", urlStr)

	var jsonStr = fmt.Sprintf(`{"channel": "%s", "username": "splitio-go-agent", "text": "%s", "icon_emoji": ":robot_face:"}`, w.Channel, p)
	req, _ := http.NewRequest("POST", urlStr, bytes.NewBuffer([]byte(jsonStr)))
	//req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		// If message has been written successfully (http 200 OK)
		return len(p), nil
	}
	return 0, fmt.Errorf("Error posting log message to Slack %s", resp.Status)
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
