// Package conf implements functions to read configuration data
package conf

import "encoding/json"

// RedisSection Redis instance information
type RedisSection struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Db     int    `json:"db"`
	Pass   string `json:"password"`
	Prefix string `json:"prefix"`
}

// LogSection log instance configuration
type LogSection struct {
	VerboseOn       bool   `json:"verbose"`
	DebugOn         bool   `json:"debug"`
	StdoutOn        bool   `json:"stdout"`
	File            string `json:"file"`
	SlackChannel    string `json:"slackChannel"`
	SlackWebhookURL string `json:"slackWebhookURL"`
}

// ConfigData main configuration container
type ConfigData struct {
	APIKey              string       `json:"apiKey"`
	Redis               RedisSection `json:"redis"`
	Logger              LogSection   `json:"log"`
	SplitsFetchRate     int          `json:"splitsRefreshRate"`
	SegmentFetchRate    int          `json:"segmentsRefreshRate"`
	ImpressionsPostRate int          `json:"impressionsRefreshRate"`
	ImpressionsPerPost  int64        `json:"impressionsPerPost"`
}

//MarshalBinary exports ConfigData to JSON string
func (c ConfigData) MarshalBinary() (data []byte, err error) {
	return json.MarshalIndent(c, "", "  ")
}

func getDefaultConfigData() ConfigData {
	configData := ConfigData{}

	//agent parameters
	configData.APIKey = "YOUR API KEY"
	configData.SplitsFetchRate = 30
	configData.SegmentFetchRate = 60
	configData.ImpressionsPostRate = 60
	configData.ImpressionsPerPost = 1000

	//logger parameters
	configData.Logger.VerboseOn = false
	configData.Logger.DebugOn = false
	configData.Logger.StdoutOn = true
	configData.Logger.File = "/tmp/split-agent.log"

	//redis parameters
	configData.Redis.Db = 0
	configData.Redis.Host = "localhost"
	configData.Redis.Pass = ""
	configData.Redis.Port = 6379
	configData.Redis.Prefix = ""

	return configData
}
