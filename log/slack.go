package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/splitio"
)

// TODO(mredolatti): Refactor this into a proper struct

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

// PostMessageToSlack post a message to Slack Channel
func PostMessageToSlack(message string, attachements []SlackMessageAttachment) {
	var slackWriter *SlackWriter

	_, err := url.ParseRequestURI(conf.Data.Logger.SlackWebhookURL)
	if err == nil {
		slackWriter = &SlackWriter{
			WebHookURL:  conf.Data.Logger.SlackWebhookURL,
			Channel:     conf.Data.Logger.SlackChannel,
			RefreshRate: 1}

	}
	if slackWriter != nil {
		slackWriter.PostNow([]byte(message), attachements)
	}
}

// PostShutdownMessageToSlack post the shutdown message to slack channel
func PostShutdownMessageToSlack(kill bool) {
	var title string
	var color string

	if kill {
		color = "danger"
	} else {
		color = "good"
	}

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		if conf.Data.Proxy.Title != "" {
			title = conf.Data.Proxy.Title
		}
	} else {
		if conf.Data.Producer.Admin.Title != "" {
			title = conf.Data.Producer.Admin.Title
		}
	}

	if title != "" {
		fields := make([]SlackMessageAttachmentFields, 0)
		fields = append(fields, SlackMessageAttachmentFields{
			Title: title,
			Value: "Shutting it down, see you soon!",
			Short: false,
		})
		attach := SlackMessageAttachment{
			Fallback: "Shutting Split-Sync down",
			Color:    color,
			Fields:   fields,
		}
		attachs := append(make([]SlackMessageAttachment, 0), attach)
		if kill {
			PostMessageToSlack("*[KILL]* Force shutdown signal sent", attachs)
		} else {
			PostMessageToSlack("*[IMPORTANT]* Starting Graceful Shutdown", attachs)
		}
	} else {
		if kill {
			PostMessageToSlack("*[KILL]* Force shutdown signal sent - see you soon!", nil)
		} else {
			PostMessageToSlack("*[IMPORTANT]* Shutting Split-Sync down - see you soon!", nil)
		}
	}
}

// PostStartedMessageToSlack post the started message to slack channel
func PostStartedMessageToSlack() {
	title := "Split-Sync"
	mode := ""

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		mode = "Proxy Mode"
		if conf.Data.Proxy.Title != "" {
			title = conf.Data.Proxy.Title
		}
	} else {
		mode = "Synchronizer Mode"
		if conf.Data.Producer.Admin.Title != "" {
			title = conf.Data.Producer.Admin.Title
		}
	}

	fields := make([]SlackMessageAttachmentFields, 0)
	fields = append(fields, SlackMessageAttachmentFields{
		Title: fmt.Sprintf("%s started", title),
		Short: false,
	})
	fields = append(fields, SlackMessageAttachmentFields{
		Title: fmt.Sprintf("Version: %s", splitio.Version),
		Short: false,
	})
	fields = append(fields, SlackMessageAttachmentFields{
		Title: fmt.Sprintf("Running as: %s", mode),
		Short: false,
	})
	attach := SlackMessageAttachment{
		Fallback: "Split-Sync started",
		Color:    "good",
		Fields:   fields,
	}
	attachs := append(make([]SlackMessageAttachment, 0), attach)
	PostMessageToSlack("*[IMPORTANT]* Split-Sync started", attachs)
}
