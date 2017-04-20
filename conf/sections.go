package conf

import "encoding/json"

// RedisSection Redis instance information
type RedisSection struct {
	Host   string `json:"host" split-default-value:"localhost" split-cli-option:"redis-host" split-cli-description:"Redis server hostname"`
	Port   int    `json:"port" split-default-value:"6379" split-cli-option:"redis-port" split-cli-description:"Redis Server port"`
	Db     int    `json:"db" split-default-value:"0" split-cli-option:"redis-db" split-cli-description:"Redis DB"`
	Pass   string `json:"password" split-default-value:"" split-cli-option:"redis-pass" split-cli-description:"Redis password"`
	Prefix string `json:"prefix" split-default-value:"" split-cli-option:"redis-prefix" split-cli-description:"Redis key prefix"`

	// The network type, either tcp or unix.
	// Default is tcp.
	Network string `json:"network" split-default-value:"tcp" split-cli-option:"redis-network" split-cli-description:"Redis network protocol"`

	// Maximum number of retries before giving up.
	// Default is to not retry failed commands.
	MaxRetries int `json:"maxRetries" split-default-value:"0" split-cli-option:"redis-max-retries" split-cli-description:"Redis connection max retries"`

	// Dial timeout for establishing new connections.
	// Default is 5 seconds.
	DialTimeout int `json:"dialTimeout" split-default-value:"5" split-cli-option:"redis-dial-timeout" split-cli-description:"Redis connection dial timeout"`

	// Timeout for socket reads. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is 10 seconds.
	ReadTimeout int `json:"readTimeout" split-default-value:"10" split-cli-option:"redis-read-timeout" split-cli-description:"Redis connection read timeout"`

	// Timeout for socket writes. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is 3 seconds.
	WriteTimeout int `json:"writeTimeout" split-default-value:"5" split-cli-option:"redis-write-timeout" split-cli-description:"Redis connection write timeout"`

	// Maximum number of socket connections.
	// Default is 10 connections.
	PoolSize int `json:"poolSize" split-default-value:"10" split-cli-option:"redis-pool" split-cli-description:"Redis connection pool size"`
}

// LogSection log instance configuration
type LogSection struct {
	VerboseOn       bool   `json:"verbose" split-default-value:"false" split-cli-option:"log-verbose" split-cli-description:"Enable verbose mode"`
	DebugOn         bool   `json:"debug" split-default-value:"false" split-cli-option:"log-debug" split-cli-description:"Enable debug mode"`
	StdoutOn        bool   `json:"stdout" split-default-value:"false" split-cli-option:"log-stdout" split-cli-description:"Enable log standard output"`
	File            string `json:"file" split-default-value:"/tmp/split-agent.log" split-cli-option:"log-file" split-cli-description:"Set the log file"`
	SlackChannel    string `json:"slackChannel" split-default-value:"" split-cli-option:"log-slack-channel" split-cli-description:"Set the Slack channel or user"`
	SlackWebhookURL string `json:"slackWebhookURL" split-default-value:"" split-cli-option:"log-slack-webhook-url" split-cli-description:"Set the Slack webhook url"`
}

// ConfigData main configuration container
type ConfigData struct {
	APIKey              string       `json:"apiKey" split-cli-option:"api-key" split-default-value:"YOUR API KEY" split-cli-description:"Your Split API-KEY"`
	Redis               RedisSection `json:"redis" split-cli-option-group:"true"`
	Logger              LogSection   `json:"log" split-cli-option-group:"true"`
	SplitsFetchRate     int          `json:"splitsRefreshRate" split-cli-option:"split-refresh-rate" split-default-value:"30" split-cli-description:"Refresh rate of splits fetcher"`
	SegmentFetchRate    int          `json:"segmentsRefreshRate" split-default-value:"60" split-cli-option:"segment-refresh-rate" split-cli-description:"Refresh rate of segments fetcher"`
	ImpressionsPostRate int          `json:"impressionsRefreshRate" split-default-value:"60" split-cli-option:"impressions-post-rate" split-cli-description:"Post rate of impressions recorder"`
	ImpressionsPerPost  int64        `json:"impressionsPerPost" split-cli-option:"impressions-per-post" split-default-value:"1000" split-cli-description:"Number of impressions to send in a POST request"`
	ImpressionsThreads  int          `json:"impressionsThreads" split-default-value:"1" split-cli-option:"impressions-recorder-threads" split-cli-description:"Number of impressions recorder threads"`
	MetricsPostRate     int          `json:"metricsRefreshRate" split-default-value:"60" split-cli-option:"metrics-post-rate" split-cli-description:"Post rate of metrics recorder"`
}

//MarshalBinary exports ConfigData to JSON string
func (c ConfigData) MarshalBinary() (data []byte, err error) {
	return json.MarshalIndent(c, "", "  ")
}
