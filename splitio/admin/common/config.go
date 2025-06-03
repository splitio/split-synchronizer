package common

import (
	pSt "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/splitio/go-split-commons/v6/storage"
)

// Storages wraps storages in one struct
type Storages struct {
	SplitStorage          storage.SplitStorage
	SegmentStorage        storage.SegmentStorage
	LocalTelemetryStorage storage.TelemetryRuntimeConsumer
	EventStorage          storage.EventMultiSdkConsumer
	ImpressionStorage     storage.ImpressionMultiSdkConsumer
	UniqueKeysStorage     storage.UniqueKeysMultiSdkConsumer
	LargeSegmentStorage   storage.LargeSegmentsStorage

	OverrideStorage pSt.OverrideStorage
}
