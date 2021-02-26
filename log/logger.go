package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v4/logging"
)

// Instance is an instance of log
var Instance logging.LoggerInterface

// ErrorDashboard is an instance of DashboardWriter
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
	c.counts++
	c.messages = append(c.messages, string(p))
	if len(c.messages) > c.messagesSize {
		c.messages = c.messages[len(c.messages)-c.messagesSize : len(c.messages)]
	}
	c.cmutex.Unlock()
	return 0, nil
}

// Counts returns the count number
func (c *DashboardWriter) Counts() int64 {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.counts
}

// Messages returns the last logged messages
func (c *DashboardWriter) Messages() []string {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.messages
}

// SlackMessageAttachmentFields attachment field struct
type SlackMessageAttachmentFields struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackMessageAttachment attach message struct
type SlackMessageAttachment struct {
	Fallback string                         `json:"fallback"`
	Text     string                         `json:"text,omitempty"`
	Pretext  string                         `json:"pretext,omitempty"`
	Color    string                         `json:"color"` // Can either be one of 'good', 'warning', 'danger', or any hex color code
	Fields   []SlackMessageAttachmentFields `json:"fields"`
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
		err := w.postMessage(w.message, nil)
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

func (w *SlackWriter) postMessage(msg []byte, attachements []SlackMessageAttachment) (err error) {
	urlStr := w.WebHookURL
	//Simple message by default
	jsonStr := fmt.Sprintf(`{"channel": "%s", "username": "Split-Sync", "text": "%s", "icon_emoji": ":robot_face:"}`, w.Channel, msg)
	if attachements != nil && len(attachements) > 0 {
		attachs, err := json.Marshal(attachements)
		if err != nil {
			fmt.Println("Error posting message to Slack with attachment fields")
		} else {
			jsonStr = fmt.Sprintf(`{"channel": "%s", "username": "Split-Sync", "text": "%s", "icon_emoji": ":robot_face:", "attachments":%s}`, w.Channel, msg, string(attachs))
		}
	}

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

// PostNow post a message directly to slack channel
func (w *SlackWriter) PostNow(msg []byte, attachements []SlackMessageAttachment) (err error) {
	return w.postMessage(msg, attachements)
}

// Initialize log module
func Initialize(
	verboseWriter io.Writer,
	debugWriter io.Writer,
	infoWriter io.Writer,
	warningWriter io.Writer,
	errorWriter io.Writer,
	level int) {

	Instance = logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: log.Ldate | log.Ltime | log.Lshortfile,
		Prefix:              "SPLITIO-AGENT ",
		VerboseWriter:       verboseWriter,
		DebugWriter:         debugWriter,
		InfoWriter:          infoWriter,
		WarningWriter:       warningWriter,
		ErrorWriter:         errorWriter,
		LogLevel:            level,
	})
}
