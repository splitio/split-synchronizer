// Package conf implements functions to read configuration data
package conf

import (
	"encoding/json"
	"fmt"
	"reflect"
)

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
	APIKey              string       `json:"apiKey" split-option:"api-key" description:"Your Split API-KEY"`
	Redis               RedisSection `json:"redis" split-option-group:"true"`
	Logger              LogSection   `json:"log"`
	SplitsFetchRate     int          `json:"splitsRefreshRate" split-option:"split-refresh-rate" description:"Refresh rate of splits fetcher"`
	SegmentFetchRate    int          `json:"segmentsRefreshRate"`
	ImpressionsPostRate int          `json:"impressionsRefreshRate"`
	ImpressionsPerPost  int64        `json:"impressionsPerPost" split-option:"impressions-per-post" description:"Number of impressions to send in a POST request"`
	ImpressionsThreads  int          `json:"impressionsThreads"`
	MetricsPostRate     int          `json:"metricsRefreshRate"`
}

//MarshalBinary exports ConfigData to JSON string
func (c ConfigData) MarshalBinary() (data []byte, err error) {
	return json.MarshalIndent(c, "", "  ")
}

//  cliParameters reflects the struct
func (c *ConfigData) cliParameters() []CommandConfigData {

	var toReturn []CommandConfigData
	val := reflect.ValueOf(c).Elem()

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		if len(tag.Get("split-option")) > 0 {
			toReturn = append(toReturn,
				CommandConfigData{
					Command:       tag.Get("split-option"),
					Description:   tag.Get("description"),
					Attribute:     typeField.Name,
					AttributeType: fmt.Sprintf("%s", typeField.Type),
					DefaultValue:  valueField.Interface()})
		}

		//fmt.Printf("Field Name: %s,\t Field Type: %s,\t Field Value: %v,\t Tag Value (option): %s,\t Tag Value (description): %s\n", typeField.Name, typeField.Type, valueField.Interface(), tag.Get("option"), tag.Get("description"))
	}
	fmt.Println(toReturn)
	return toReturn

	//var dataAK = flag.String("api-key", c.APIKey, "API KEY!!!")
	//flag.Parse()
	//val.FieldByName("APIKey").SetString(*dataAK)
	//fmt.Println((val.FieldByName("Redis").Interface().(RedisSection)).Host)

}

// CommandConfigData represent a command line data structure
type CommandConfigData struct {
	Command       string
	Description   string
	Attribute     string
	AttributeType string
	DefaultValue  interface{}
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
