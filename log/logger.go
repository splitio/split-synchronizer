package log

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	// Benchmark level
	Benchmark *log.Logger
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

// ErrorDashboard
var ErrorDashboard = &DashboardWriter{cmutex: &sync.Mutex{}, counts: 0, messages: make([]string, 0), messagesSize: 10}

// DashboardWriter counts each call to Write method
type DashboardWriter struct {
	counts       int64
	cmutex       *sync.Mutex
	messages     []string
	messagesSize int
}

func (c *DashboardWriter) Write(p []byte) (n int, err error) {
	c.cmutex.Lock()
	c.counts += 1
	c.messages = append(c.messages, string(p))
	if len(c.messages) > c.messagesSize {
		c.messages = c.messages[len(c.messages)-c.messagesSize : len(c.messages)]
	}
	c.cmutex.Unlock()
	return 0, nil
}

func (c *DashboardWriter) Counts() int64 {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.counts
}

func (c *DashboardWriter) Messages() []string {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.messages
}

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
func Initialize(benchmarkWriter io.Writer, verboseWriter io.Writer,
	debugWriter io.Writer,
	infoWriter io.Writer,
	warningWriter io.Writer,
	errorWriter io.Writer) {

	Benchmark = log.New(benchmarkWriter,
		"SPLITIO-AGENT | BENCHMARK: ",
		log.Ldate|log.Ltime|log.Lshortfile)

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

	errWriter := io.MultiWriter(errorWriter, ErrorDashboard)
	Error = log.New(errWriter,
		"SPLITIO-AGENT | ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}
