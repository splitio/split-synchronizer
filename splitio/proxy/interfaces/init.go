package interfaces

import (
	"github.com/splitio/go-split-commons/v3/service"
	"github.com/splitio/go-split-commons/v3/service/api"
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-split-commons/v3/storage/mutexmap"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	storageV2 "github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/v2"
	v2 "github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/v2"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
)

// TelemetryStorage storage
var TelemetryStorage *mutexmap.MMMetricsStorage

// ProxyTelemetryWrapper telemetry
var ProxyTelemetryWrapper *storage.MetricWrapper

// MetricsRecorder recorder
var MetricsRecorder *api.HTTPMetricsRecorder

// SegmentStorage storage
var SegmentStorage *mutexmap.MMSegmentStorage

// MySegmentsCache cache
var MySegmentsCache *v2.MySegmentsCache

// SplitChangesSummary storage
var SplitChangesSummary *storageV2.SplitChangesSummaries

// SplitStorage storage
var SplitStorage *mutexmap.MMSplitStorage

// SplitAPI api
var SplitAPI *service.SplitAPI

// Initialize Wrappers
func Initialize() {
	if TelemetryStorage == nil {
		TelemetryStorage = mutexmap.NewMMMetricsStorage()
	}

	if ProxyTelemetryWrapper == nil {
		ProxyTelemetryWrapper = storage.NewMetricWrapper(TelemetryStorage, mutexmap.NewMMMetricsStorage(), log.Instance)
	}

	if MetricsRecorder == nil {
		MetricsRecorder = api.NewHTTPMetricsRecorder(conf.Data.APIKey, conf.ParseAdvancedOptions(), log.Instance)
	}

	if SegmentStorage == nil {
		SegmentStorage = mutexmap.NewMMSegmentStorage()
	}

	if MySegmentsCache == nil {
		MySegmentsCache = v2.NewMySegmentsCache()
	}

	if SplitChangesSummary == nil {
		SplitChangesSummary = storageV2.NewSplitChangesSummaries()
	}

	if SplitStorage == nil {
		SplitStorage = mutexmap.NewMMSplitStorage()
	}

	if SplitAPI == nil {
		advanced := conf.ParseAdvancedOptions()
		metadata := util.GetMetadata()
		SplitAPI = service.NewSplitAPI(conf.Data.APIKey, advanced, log.Instance, metadata)
	}
}
