package log

import (
	"net/url"

	"github.com/splitio/split-synchronizer/conf"
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
