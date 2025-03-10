package conf

// Logging configuration options
type Logging struct {
	Level             string `json:"level" s-cli:"log-level" s-def:"info" s-desc:"Log level (error|warning|info|debug|verbose)"`
	Output            string `json:"output" s-cli:"log-output" s-def:"stdout" s-desc:"Where to output logs (defaults to stdout)"`
	RotationMaxFiles  int64  `json:"rotationMaxFiles" s-cli:"log-rotation-max-files" s-def:"10" s-desc:"Max number of files to keep when rotating logs"`
	RotationMaxSizeKb int64  `json:"rotationMaxSizeKb" s-cli:"log-rotation-max-size-kb" s-def:"1024" s-desc:"Maximum log file size in kbs"`
}

// Admin configuration options
type Admin struct {
	Host     string `json:"host" s-cli:"admin-host" s-def:"0.0.0.0" s-desc:"Host where the admin server will listen"`
	Port     int64  `json:"port" s-cli:"admin-port" s-def:"3010" s-desc:"Admin port where incoming connections will be accepted"`
	Username string `json:"username" s-cli:"admin-username" s-def:"" s-desc:"HTTP basic auth username for admin endpoints"`
	Password string `json:"password" s-cli:"admin-password" s-def:"" s-desc:"HTTP basic auth password for admin endpoints"`
	SecureHC bool   `json:"secureChecks" s-cli:"admin-secure-hc" s-def:"false" s-desc:"Secure Healthcheck endpoints as well."`
	TLS      TLS    `json:"tls" s-nested:"true" s-cli-prefix:"admin"`
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

// TLS config options
type TLS struct {
	Enabled                  bool   `json:"enabled" s-cli:"tls-enabled" s-def:"false" s-desc:"Enable HTTPS on proxy endpoints"`
	ClientValidation         bool   `json:"clientValidation" s-cli:"tls-client-validation" s-def:"false" s-desc:"Enable client cert validation"`
	ServerName               string `json:"serverName" s-cli:"tls-server-name" s-def:"" s-desc:"Server name as it appears in provided server-cert"`
	CertChainFN              string `json:"certChainFn" s-cli:"tls-cert-chain-fn" s-def:"" s-desc:"X509 Server certificate chain"`
	PrivateKeyFN             string `json:"privateKeyFn" s-cli:"tls-private-key-fn" s-def:"" s-desc:"PEM Private key file name"`
	ClientValidationRootCert string `json:"clientValidationRootCertFn" s-cli:"tls-client-validation-root-cert" s-def:"" s-desc:"X509 root cert for client validation"`
	MinTLSVersion            string `json:"minTlsVersion" s-cli:"tls-min-tls-version" s-def:"1.3" s-desc:"Minimum TLS version to allow X.Y"`
	AllowedCipherSuites      string `json:"allowedCipherSuites" s-cli:"tls-allowed-cipher-suites" s-def:"" s-desc:"Comma-separated list of cipher suites to allow"`
}
