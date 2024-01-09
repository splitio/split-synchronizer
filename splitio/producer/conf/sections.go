package conf

import (
	cconf "github.com/splitio/go-split-commons/v5/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

// Main configuration options
type Main struct {
	Apikey           string            `json:"apikey" s-cli:"apikey" s-def:"" s-desc:"Split server side SDK key"`
	IPAddressEnabled bool              `json:"ipAddressEnabled" s-cli:"ip-address-enabled" s-def:"true" s-desc:"Bundle host's ip address when sending data to Split"`
	FlagSetsFilter   []string          `json:"flagSetsFilter" s-cli:"flag-sets-filter" s-def:"" s-desc:"Flag Sets Filter provided"`
	Initialization   Initialization    `json:"initialization" s-nested:"true"`
	Storage          Storage           `json:"storage" s-nested:"true"`
	Sync             Sync              `json:"sync" s-nested:"true"`
	Admin            conf.Admin        `json:"admin" s-nested:"true"`
	Integrations     conf.Integrations `json:"integrations" s-nested:"true"`
	Logging          conf.Logging      `json:"logging" s-nested:"true"`
	Healthcheck      Healthcheck       `json:"healthcheck" s-nested:"true"`
}

// BuildAdvancedConfig generates a commons-compatible advancedconfig with default + overriden parameters
func (m *Main) BuildAdvancedConfig() *cconf.AdvancedConfig {
	tmp := conf.InitAdvancedOptions(false) // defaults + url overrides
	tmp.HTTPTimeout = int(m.Sync.Advanced.HTTPTimeoutMs / 1000)
	tmp.StreamingEnabled = m.Sync.Advanced.StreamingEnabled
	tmp.SplitsRefreshRate = int(m.Sync.SplitRefreshRateMs / 1000)
	tmp.SegmentsRefreshRate = int(m.Sync.SegmentRefreshRateMs / 1000)
	return tmp
}

// Initialization configuration options
type Initialization struct {
	TimeoutMs int64 `json:"timeoutMS" s-cli:"timeout-ms" s-def:"10000" s-desc:"How long to wait until the synchronizer is ready"`
	// Coming soon
	// Snapshot          string `json:"snapshot" s-cli:"snapshot" s-def:"" s-desc:"Snapshot file to use as a starting point"`
	ForceFreshStartup bool `json:"forceFreshStartup" s-cli:"force-fresh-startup" s-def:"false" s-desc:"Wipe storage before starting the synchronizer"`
}

// Storage configuration options
type Storage struct {
	Type  string `json:"type" s-cli:"storage-type" s-def:"redis" s-desc:"Storage driver to use for caching feature flags/segments and user-generated data"`
	Redis Redis  `json:"redis" s-nested:"true"`
}

// Sync configuration options
type Sync struct {
	SplitRefreshRateMs   int64        `json:"splitRefreshRateMs" s-cli:"split-refresh-rate-ms" s-def:"60000" s-desc:"How often to refresh feature flags"`
	SegmentRefreshRateMs int64        `json:"segmentRefreshRateMs" s-cli:"segment-refresh-rate-ms" s-def:"60000" s-desc:"How often to refresh segments"`
	ImpressionsMode      string       `json:"impressionsMode" s-cli:"impressions-mode" s-def:"optimized" s-desc:"whether to send all impressions for debugging"`
	Advanced             AdvancedSync `json:"advanced" s-nested:"true"`
}

// AdvancedSync configuration options
type AdvancedSync struct {
	StreamingEnabled                 bool  `json:"streamingEnabled" s-cli:"streaming-enabled" s-def:"true" s-desc:"Enable/disable streaming functionality"`
	HTTPTimeoutMs                    int64 `json:"httpTimeoutMs" s-cli:"http-timeout-ms" s-def:"30000" s-desc:"Total http request timeout"`
	InternalMetricsRateMs            int64 `json:"internalTelemetryRateMs" s-cli:"internal-metrics-rate-ms" s-def:"3600000" s-desc:"How often to send internal metrics"`
	TelemetryPushRateMs              int64 `json:"telemetryPushRateMs" s-cli:"telemetry-push-rate-ms" s-def:"60000" s-desc:"how often to flush sdk telemetry"`
	ImpressionsFetchSize             int64 `json:"impressionsFetchSize" s-cli:"impressions-fetch-size" s-def:"0" s-desc:"Impression fetch bulk size"`
	ImpressionsProcessConcurrency    int   `json:"impressionsProcessConcurrency" s-cli:"impressions-process-concurrency" s-def:"0" s-desc:"#Threads for processing imps"`
	ImpressionsProcessBatchSize      int   `json:"impressionsProcessBatchSize" s-cli:"impressions-process-batch-size" s-def:"0" s-desc:"Size of imp processing batchs"`
	ImpressionsPostConcurrency       int   `json:"impressionsPostConcurrency" s-cli:"impressions-post-concurrency" s-def:"0" s-desc:"#concurrent imp post threads"`
	ImpressionsPostSize              int   `json:"impressionsPostSize" s-cli:"impressions-post-size" s-def:"0" s-desc:"Max #impressions to send per POST"`
	ImpressionsAccumWaitMs           int64 `json:"impressionsAccumWaitMs" s-cli:"impressions-accum-wait-ms" s-def:"0" s-desc:"Max ms to wait to close an impressions bulk"`
	EventsFetchSize                  int64 `json:"eventsFetchSize" s-cli:"events-fetch-size" s-def:"0" s-desc:"How many impressions to pop from storage at once"`
	EventsProcessConcurrency         int   `json:"eventsProcessConcurrency" s-cli:"events-process-concurrency" s-def:"0" s-desc:"#Threads for processing imps"`
	EventsProcessBatchSize           int   `json:"eventsProcessBatchSize" s-cli:"events-process-batch-size" s-def:"0" s-desc:"Size of imp processing batchs"`
	EventsPostConcurrency            int   `json:"eventsPostConcurrency" s-cli:"events-post-concurrency" s-def:"0" s-desc:"#concurrent imp post threads"`
	EventsPostSize                   int   `json:"eventsPostSize" s-cli:"events-post-size" s-def:"0" s-desc:"Max #impressions to send per POST"`
	EventsAccumWaitMs                int64 `json:"eventsAccumWaitMs" s-cli:"events-accum-wait-ms" s-def:"0" s-desc:"Max ms to wait to close an events bulk"`
	UniqueKeysFetchSize              int64 `json:"uniqueKeysFetchSize" s-cli:"unique-keys-fetch-size" s-def:"0" s-desc:"How many unique keys to pop from storage at once"`
	UniqueKeysProcessConcurrency     int   `json:"uniqueKeysProcessConcurrency" s-cli:"unique-keys-process-concurrency" s-def:"0" s-desc:"#Threads for processing uniques"`
	UniqueKeysProcessBatchSize       int   `json:"uniqueKeysProcessBatchSize" s-cli:"unique-keys-process-batch-size" s-def:"0" s-desc:"Size of uniques processing batchs"`
	UniqueKeysPostConcurrency        int   `json:"uniqueKeysPostConcurrency" s-cli:"unique-keys-post-concurrency" s-def:"0" s-desc:"#concurrent uniques post threads"`
	UniqueKeysAccumWaitMs            int64 `json:"uniqueKeysAccumWaitMs" s-cli:"unique-keys-accum-wait-ms" s-def:"0" s-desc:"Max ms to wait to close an uniques bulk"`
	ImpressionsCountWorkerReadRateMs int64 `json:"impressionsCountWorkerReadRateMs" s-cli:"impressions-count-worker-read-rate-ms" s-def:"60000" s-desc:"how often read in redis impression count comming from sdks"`
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
	ScanCount             int      `json:"scanCount" s-cli:"redis-scan-count" s-def:"10" s-desc:"It is the number of keys to search through at a time per cursor iteration, we use it to read feature flag names and flag set names."`
}

// Healthcheck configuration options
type Healthcheck struct {
	App HealthcheckApp `json:"app" s-nested:"true"`
}

// HealthcheckApp configuration options
type HealthcheckApp struct {
	StorageCheckRateMs int64 `json:"storageCheckRateMs" s-cli:"storage-check-rate-ms" s-def:"3600000" s-desc:"How often to check storage health"`
}
