package log

import (
	"fmt"
	"net/url"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio"
)

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
