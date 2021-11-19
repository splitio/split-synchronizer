package conf

// Logging configuration options
type Logging struct {
	Level            string `json:"level" s-cli:"log-level" s-def:"info" s-desc:"Log level (error|warning|info|debug|verbose)"`
	Output           string `json:"output" s-cli:"log-output" s-def:"stdout" s-desc:"Where to output logs (defaults to stdout)"`
	RotationMaxFiles int64  `json:"rotationMaxFiles" s-cli:"log-rotation-max-files" s-def:"10" s-desc:"Max number of files to keep when rotating logs"`
	RotationMaxSize  int64  `json:"rotationMaxSizeKb" s-cli:"log-rotation-max-size-kb" s-def:"Maximum log file size in kbs"`
}

// Admin configuration options
type Admin struct {
	Host     string `json:"host" s-cli:"admin-host" s-def:"0.0.0.0" s-desc:"Host where the admin server will listen"`
	Port     int64  `json:"port" s-cli:"admin-port" s-def:"3010" s-desc:"Admin port where incoming connections will be accepted"`
	Username string `json:"username" s-cli:"admin-username" s-def:"" s-desc:"HTTP basic auth username for admin endpoints"`
	Password string `json:"password" s-cli:"admin-password" s-def:"" s-desc:"HTTP basic auth password for admin endpoints"`
	SecureHC bool   `json:"secureChecks" s-cli:"admin-secure-hc" s-def:"false" s-desc:"Secure Healthcheck endpoints as well."`
}

// Integrations configuration options
type Integrations struct {
	ImpressionListener ImpressionListener `json:"impressionListener" s-nested:"true"`
	Slack              Slack              `json:"slack" s-nested:"true"`
}

// ImpressionListener configuration options
type ImpressionListener struct {
	Endpoint  string `json:"endpoint" s-cli:"impression-listener-endpoint" s-def:"" s-desc:"HTTP endpoint to forward impressions to"`
	QueueSize int64  `json:"queueSize" s-cli:"impression-listener-queue-size" s-def:"100" s-desc:"max number of impressions bulks to queue"`
}

// Slack configuration options
type Slack struct {
	Webhook string `json:"webhook" s-cli:"slack-webhook" s-def:"" s-desc:"slack webhook to post log messages"`
	Channel string `json:"channel" s-cli:"slack-channel" s-def:"" s-desc:"slack channel to post log messages"`
}
