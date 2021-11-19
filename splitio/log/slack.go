package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

// SlackWriter writes messages to Slack user or channel. Implements io.Writer interface
type SlackWriter struct {
	webhookURL string
	httpClient http.Client
	channel    string
	buffer     chan []byte
	lastSent   time.Time
}

// NewSlackWriter constructs a slack writer
func NewSlackWriter(webhookURL string, channel string) *SlackWriter {
	toRet := &SlackWriter{
		webhookURL: webhookURL,
		channel:    channel,
		buffer:     make(chan []byte, 200),
	}

	go toRet.poster()
	return toRet
}

// Write the message to slack webhook
func (w *SlackWriter) Write(p []byte) (n int, err error) {
	message := make([]byte, len(p))
	copy(message, p)

	select {
	case w.buffer <- message:
	default:
		// println a message?
	}
	return len(p), nil
}

func (w *SlackWriter) poster() {
	timer := time.NewTimer(500 * time.Millisecond) // TODO(mredolatti): make this configurable in final release
	localBuffer := make([][]byte, 0, 20)
	for {
		select {
		// TODO(mredolatti): add an exit path that flushes all messages on shutdown
		case message := <-w.buffer:
			localBuffer = append(localBuffer, message)
		case <-timer.C:
			for _, message := range localBuffer {
				w.postMessage(message, nil)
			}
			localBuffer = localBuffer[:0] // reset the slice without releasing/reallocating memory
			timer.Reset(500 * time.Millisecond)
		}
	}
}

func (w *SlackWriter) postMessage(msg []byte, attachements []SlackMessageAttachment) (err error) {
	message := messagePayload{
		Channel:     w.channel,
		Username:    "Split-Sync",
		Text:        string(msg),
		IconEmoji:   ":robot_face:",
		Attachments: attachements,
	}

	serialized, err := json.Marshal(&message)
	if err != nil {
		return fmt.Errorf("error serializing message: %w", err)
	}

	req, _ := http.NewRequest("POST", w.webhookURL, bytes.NewBuffer(serialized))
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error posting log message to slack: %w", err)
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

type messagePayload struct {
	Channel     string `json:"channel"`
	Username    string `json:"username"`
	Text        string `json:"text"`
	IconEmoji   string `json:"icon_emoji"`
	Attachments []SlackMessageAttachment
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

var _ io.Writer = (*SlackWriter)(nil)
