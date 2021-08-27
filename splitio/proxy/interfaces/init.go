package interfaces

// import (
// 	"github.com/splitio/split-synchronizer/v4/splitio/common/telemetry"
// )
//
// // LocalTelemetry exposes local telemetry TODO(mredolatti): Remove this global instance
// var LocalTelemetry telemetry.ProxyTelemetryFacade = telemetry.NewProxyTelemetryFacade()
//
// // import (
// // 	"github.com/splitio/go-split-commons/v4/service/api"
// // 	"github.com/splitio/go-split-commons/v4/storage"
// // 	"github.com/splitio/go-split-commons/v4/storage/mutexmap"
// // 	"github.com/splitio/split-synchronizer/v4/conf"
// // 	"github.com/splitio/split-synchronizer/v4/log"
// // )
// //
// // // TelemetryStorage storage
// // var TelemetryStorage *mutexmap.MMMetricsStorage
// //
// // // ProxyTelemetryWrapper telemetry
// // var ProxyTelemetryWrapper *storage.MetricWrapper
// //
// // // MetricsRecorder recorder
// // var MetricsRecorder *api.HTTPMetricsRecorder
//
// // Initialize Wrappers
// func Initialize() {
// 	// 	if TelemetryStorage == nil {
// 	// 		TelemetryStorage = mutexmap.NewMMMetricsStorage()
// 	// 	}
// 	//
// 	// 	if ProxyTelemetryWrapper == nil {
// 	// 		ProxyTelemetryWrapper = storage.NewMetricWrapper(TelemetryStorage, mutexmap.NewMMMetricsStorage(), log.Instance)
// 	// 	}
// 	//
// 	// 	if MetricsRecorder == nil {
// 	// 		MetricsRecorder = api.NewHTTPMetricsRecorder(conf.Data.APIKey, conf.ParseAdvancedOptions(), log.Instance)
// 	// 	}
// }
