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

	// The network type, either tcp or unix.
	// Default is tcp.
	Network string `json:"network"`

	// Maximum number of retries before giving up.
	// Default is to not retry failed commands.
	MaxRetries int `json:"maxRetries"`

	// Dial timeout for establishing new connections.
	// Default is 5 seconds.
	DialTimeout int `json:"dialTimeout"`

	// Timeout for socket reads. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is 10 seconds.
	ReadTimeout int `json:"readTimeout"`

	// Timeout for socket writes. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is 3 seconds.
	WriteTimeout int `json:"writeTimeout"`

	// Maximum number of socket connections.
	// Default is 10 connections.
	PoolSize int `json:"poolSize"`
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
	ImpressionsThreads  int          `json:"impressionsThreads"`
	MetricsPostRate     int          `json:"metricsRefreshRate"`
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
	configData.ImpressionsThreads = 1
	configData.MetricsPostRate = 60

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
	configData.Redis.Network = "tcp"
	configData.Redis.DialTimeout = 5
	configData.Redis.MaxRetries = 0
	configData.Redis.PoolSize = 10
	configData.Redis.ReadTimeout = 10
	configData.Redis.WriteTimeout = 5

	return configData
}
