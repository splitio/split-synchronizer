package conf

import (
	cconf "github.com/splitio/go-split-commons/v6/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

// Main configuration options
type Main struct {
	Apikey                string            `json:"apikey" s-cli:"apikey" s-def:"" s-desc:"Split server side SDK key"`
	IPAddressEnabled      bool              `json:"ipAddressEnabled" s-cli:"ip-address-enabled" s-def:"true" s-desc:"Bundle host's ip address when sending data to Split"`
	FlagSetsFilter        []string          `json:"flagSetsFilter" s-cli:"flag-sets-filter" s-def:"" s-desc:"Flag Sets Filter provided"`
	FlagSetStrictMatching bool              `json:"flagSetStrictMatching" s-cli:"flag-sets-strict-matching" s-def:"false" s-desc:"filter sets not present in cache when building splitChanges responses"`
	Initialization        Initialization    `json:"initialization" s-nested:"true"`
	Server                Server            `json:"server" s-nested:"true"`
	Admin                 conf.Admin        `json:"admin" s-nested:"true"`
	Storage               Storage           `json:"storage" s-nested:"true"`
	Sync                  Sync              `json:"sync" s-nested:"true"`
	Integrations          conf.Integrations `json:"integrations" s-nested:"true"`
	Logging               conf.Logging      `json:"logging" s-nested:"true"`
	Healthcheck           Healthcheck       `json:"healthcheck" s-nested:"true"`
	Observability         Observability     `json:"observability" s-nested:"true"`
	FlagSpecVersion       string            `json:"flagSpecVersion" s-cli:"flag-spec-version" s-def:"1.2" s-desc:"Spec version for flags"`
	LargeSegmentVersion   string            `json:"largeSegmentVersion" s-cli:"large-segment-version" s-def:"1.0" s-desc:"Spec version for large segments"`
}

// BuildAdvancedConfig generates a commons-compatible advancedconfig with default + overriden parameters
func (m *Main) BuildAdvancedConfig() *cconf.AdvancedConfig {
	tmp := conf.InitAdvancedOptions(true) // defaults + url overrides
	tmp.HTTPTimeout = int(m.Sync.Advanced.HTTPTimeoutMs / 1000)
	tmp.ImpressionsQueueSize = int(m.Sync.Advanced.ImpressionsBuffer / 1000)
	tmp.EventsQueueSize = int(m.Sync.Advanced.EventsBuffer)
	tmp.StreamingEnabled = m.Sync.Advanced.StreamingEnabled
	tmp.SplitsRefreshRate = int(m.Sync.SplitRefreshRateMs / 1000)
	tmp.SegmentsRefreshRate = int(m.Sync.SegmentRefreshRateMs / 1000)
	return tmp
}

// Initialization configuration options
type Initialization struct {
	TimeoutMs         int64  `json:"timeoutMS" s-cli:"timeout-ms" s-def:"10000" s-desc:"How long to wait until the synchronizer is ready"`
	Snapshot          string `json:"snapshot" s-cli:"snapshot" s-def:"" s-desc:"Snapshot file to use as a starting point"`
	ForceFreshStartup bool   `json:"forceFreshStartup" s-cli:"force-fresh-startup" s-def:"false" s-desc:"Wipe storage before starting the synchronizer"`
}

// Server configuration options
type Server struct {
	ClientApikeys []string `json:"apikeys" s-cli:"client-apikeys" s-def:"SDK_API_KEY" s-desc:"Apikeys that clients connecting to this proxy will use."`
	Host          string   `json:"host" s-cli:"server-host" s-def:"0.0.0.0" s-desc:"Host/IP to start the proxy server on"`
	Port          int64    `json:"port" s-cli:"server-port" s-def:"3000" s-desc:"Port to listten for incoming requests from SDKs"`
	CacheSize     int64    `json:"httpCacheSize" s-cli:"http-cache-size" s-def:"1000000" s-desc:"How many responses to cache"`
	TLS           conf.TLS `json:"tls" s-nested:"true" s-cli-prefix:"server"`
}

// Storage configuration options
type Storage struct {
	Volatile   Volatile   `json:"volatile" s-nested:"true"`
	Persistent Persistent `json:"persistent" s-nested:"true"`
}

// Volatile storage configuration options
type Volatile struct {
}

// Persistent storage configuration options
type Persistent struct {
	Filename string `json:"filename" s-cli:"persistent-storage-fn" s-def:"" s-desc:"Where to store flags & user-generated data. (Default: temporary file)"`
}

// Sync configuration options
type Sync struct {
	SplitRefreshRateMs        int64        `json:"splitRefreshRateMs" s-cli:"split-refresh-rate-ms" s-def:"60000" s-desc:"How often to refresh feature flags"`
	SegmentRefreshRateMs      int64        `json:"segmentRefreshRateMs" s-cli:"segment-refresh-rate-ms" s-def:"60000" s-desc:"How often to refresh segments"`
	LargeSegmentRefreshRateMs int64        `json:"largeSegmentRefreshRateMs" s-cli:"large-segment-refresh-rate-ms" s-def:"3600000" s-desc:"How often to refresh large segments"`
	Advanced                  AdvancedSync `json:"advanced" s-nested:"true"`
}

// AdvancedSync configuration options
type AdvancedSync struct {
	StreamingEnabled      bool  `json:"streamingEnabled" s-cli:"streaming-enabled" s-def:"true" s-desc:"Enable/disable streaming functionality"`
	HTTPTimeoutMs         int64 `json:"httpTimeoutMs" s-cli:"http-timeout-ms" s-def:"30000" s-desc:"Total http request timeout"`
	ImpressionsBuffer     int64 `json:"impressionsBufferSize" s-cli:"impressions-buffer-size" s-def:"500" s-dec:"How many impressions bulks to keep in memory"`
	EventsBuffer          int64 `json:"eventsBufferSize" s-cli:"events-buffer-size" s-def:"500" s-dec:"How many events bulks to keep in memory"`
	TelemetryBuffer       int64 `json:"telemetryBufferSize" s-cli:"telemetry-buffer-size" s-def:"500" s-dec:"How many telemetry bulks to keep in memory"`
	ImpressionsWorkers    int64 `json:"impressionsWorkers" s-cli:"impressions-workers" s-def:"10" s-desc:"#workers to forward impressions to Split servers"`
	EventsWorkers         int64 `json:"eventsWorkers" s-cli:"events-workers" s-def:"10" s-desc:"#workers to forward events to Split servers"`
	TelemetryWorkers      int64 `json:"telemetryWorkers" s-cli:"telemetry-workers" s-def:"10" s-desc:"#workers to forward telemetry to Split servers"`
	InternalMetricsRateMs int64 `json:"internalTelemetryRateMs" s-cli:"internal-metrics-rate-ms" s-def:"3600000" s-desc:"How often to send internal metrics"`
}

// Healthcheck configuration options
type Healthcheck struct {
	Dependecies HealthcheckDependecines `json:"dependencies" s-nested:"true"`
}

// HealthcheckDependecines configuration options
type HealthcheckDependecines struct {
	DependenciesCheckRateMs int64 `json:"dependenciesCheckRateMs" s-cli:"dependencies-check-rate-ms" s-def:"3600000" s-desc:"How often to check dependecies health"`
}

// Observability configuration options
type Observability struct {
	TimeSliceWidthSecs int64 `json:"timeSliceWidthSecs" s-cli:"observability-time-slice-width-secs" s-def:"300" s-desc:"time slice size in seconds"`
	MaxTimeSliceCount  int64 `json:"maxTimeSliceCount" s-cli:"observability-time-slice-max-count" s-def:"100" s-desc:"max time slices to keep in memory before rotating"`
}
