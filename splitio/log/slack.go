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
	webhookURL  string
	httpClient  http.Client
	channel     string
	refreshRate time.Duration
	message     []byte
	lastSent    time.Time
}

// NewSlackWriter constructs a slack writer
func NewSlackWriter(webhookURL string, channel string, refreshRate time.Duration) *SlackWriter {
	return &SlackWriter{
		webhookURL:  webhookURL,
		channel:     channel,
		refreshRate: refreshRate,
	}
}

// Write the message to slack webhook
func (w *SlackWriter) Write(p []byte) (n int, err error) {
	w.message = append(w.message, p...)
	currentTime := time.Now()
	gapTime := currentTime.Sub(w.lastSent)
	if gapTime >= w.refreshRate {
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
