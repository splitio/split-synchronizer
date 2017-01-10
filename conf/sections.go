// Package conf implements functions to read configuration data
package conf

// ServerSection servers information
type ServerSection struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

// RedisSection Redis instance information
type RedisSection struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Db   int    `json:"db"`
	Pass string `json:"password"`
}

// LogSection log instance configuration
type LogSection struct {
	VerboseOn bool   `json:"verbose"`
	DebugOn   bool   `json:"debug"`
	StdoutOn  bool   `json:"stdout"`
	File      string `json:"file"`
}

// ConfigData main configuration container
type ConfigData struct {
	APIKey     string          `json:"api-key"`
	APIServers []ServerSection `json:"api-servers"`
	Redis      RedisSection    `json:"redis"`
	Logger     LogSection      `json:"log"`
}
