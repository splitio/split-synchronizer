package conf

import (
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"

	cconf "github.com/splitio/go-split-commons/v10/conf"
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
	FlagSpecVersion  string            `json:"flagSpecVersion" s-cli:"flag-spec-version" s-def:"1.3" s-desc:"Spec version for flags"`
}

// BuildAdvancedConfig generates a commons-compatible advancedconfig with default + overriden parameters
func (m *Main) BuildAdvancedConfig() *cconf.AdvancedConfig {
	tmp := conf.InitAdvancedOptions(false) // defaults + url overrides
	tmp.HTTPTimeout = int(m.Sync.Advanced.HTTPTimeoutMs / 1000)
	tmp.StreamingEnabled = m.Sync.Advanced.StreamingEnabled
	tmp.StreamingForceHTTP1 = m.Sync.Advanced.StreamingForceHTTP1
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
	Type  string     `json:"type" s-cli:"storage-type" s-def:"redis" s-desc:"Storage driver to use for caching feature flags/segments and user-generated data"`
	Redis conf.Redis `json:"redis" s-nested:"true"`
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
	StreamingForceHTTP1              bool  `json:"streamingForceHttp1" s-cli:"streaming-force-http1" s-def:"false" s-desc:"Pin the streaming (SSE) connection to HTTP/1.1 (default is HTTP/2)"`
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

// Healthcheck configuration options
type Healthcheck struct {
	App HealthcheckApp `json:"app" s-nested:"true"`
}

// HealthcheckApp configuration options
type HealthcheckApp struct {
	StorageCheckRateMs int64 `json:"storageCheckRateMs" s-cli:"storage-check-rate-ms" s-def:"3600000" s-desc:"How often to check storage health"`
}
