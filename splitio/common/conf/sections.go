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

// Redis configuration options
type Redis struct {
	Host                  string   `json:"host" s-cli:"redis-host" s-def:"localhost" s-desc:"Redis server hostname"`
	Port                  int      `json:"port" s-cli:"redis-port" s-def:"6379" s-desc:"Redis Server port"`
	Db                    int      `json:"db" s-cli:"redis-db" s-def:"0" s-desc:"Redis DB"`
	Username              string   `json:"username" s-cli:"redis-user" s-def:"" s-desc:"Redis username"`
	Pass                  string   `json:"password" s-cli:"redis-pass" s-def:"" s-desc:"Redis password"`
	Prefix                string   `json:"prefix" s-cli:"redis-prefix" s-def:"" s-desc:"Redis key prefix"`
	Network               string   `json:"network" s-cli:"redis-network" s-def:"tcp" s-desc:"Redis network protocol"`
	MaxRetries            int      `json:"maxRetries" s-cli:"redis-max-retries" s-def:"0" s-desc:"Redis connection max retries"`
	DialTimeout           int      `json:"dialTimeout" s-cli:"redis-dial-timeout" s-def:"5" s-desc:"Redis connection dial timeout"`
	ReadTimeout           int      `json:"readTimeout" s-cli:"redis-read-timeout" s-def:"10" s-desc:"Redis connection read timeout"`
	WriteTimeout          int      `json:"writeTimeout" s-cli:"redis-write-timeout" s-def:"5" s-desc:"Redis connection write timeout"`
	PoolSize              int      `json:"poolSize" s-cli:"redis-pool" s-def:"10" s-desc:"Redis connection pool size"`
	SentinelReplication   bool     `json:"sentinelReplication" s-cli:"redis-sentinel-replication" s-def:"false" s-desc:"Redis sentinel replication enabled."`
	SentinelAddresses     string   `json:"sentinelAddresses" s-cli:"redis-sentinel-addresses" s-def:"" s-desc:"List of redis sentinels"`
	SentinelMaster        string   `json:"sentinelMaster" s-cli:"redis-sentinel-master" s-def:"" s-desc:"Name of master"`
	ClusterMode           bool     `json:"clusterMode" s-cli:"redis-cluster-mode" s-def:"false" s-desc:"Redis cluster enabled."`
	ClusterNodes          string   `json:"clusterNodes" s-cli:"redis-cluster-nodes" s-def:"" s-desc:"List of redis cluster nodes."`
	ClusterKeyHashTag     string   `json:"keyHashTag" s-cli:"redis-cluster-key-hashtag" s-def:"" s-desc:"keyHashTag for redis cluster."`
	TLS                   bool     `json:"enableTLS" s-cli:"redis-tls" s-def:"false" s-desc:"Use SSL/TLS for connecting to redis"`
	TLSServerName         string   `json:"tlsServerName" s-cli:"redis-tls-server-name" s-def:"" s-desc:"Server name to use when validating a server public key"`
	TLSCACertificates     []string `json:"caCertificates" s-cli:"redis-tls-ca-certs" s-def:"" s-desc:"Root CA certificates to connect to a redis server via SSL/TLS"`
	TLSSkipNameValidation bool     `json:"tlsSkipNameValidation" s-cli:"redis-tls-skip-name-validation" s-def:"false" s-desc:"Blindly accept server's public key."`
	TLSClientCertificate  string   `json:"tlsClientCertificate" s-cli:"redis-tls-client-certificate" s-def:"" s-desc:"Client certificate signed by a known CA"`
	TLSClientKey          string   `json:"tlsClientKey" s-cli:"redis-tls-client-key" s-def:"" s-desc:"Client private key matching the certificate."`
}
