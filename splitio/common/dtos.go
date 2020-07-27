package common

import (
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/synchronizer/worker/impression"
)

// Storages struct
type Storages struct {
	SplitStorage          storage.SplitStorage
	SegmentStorage        storage.SegmentStorage
	LocalTelemetryStorage storage.MetricsStorage
	TelemetryStorage      storage.MetricsStorage
	EventStorage          storage.EventsStorage
	ImpressionStorage     storage.ImpressionStorage
}

// HTTPClients struct
type HTTPClients struct {
	SdkClient    api.Client
	EventsClient api.Client
}

// Recorders struct
type Recorders struct {
	Impression impression.ImpressionRecorder
	Event      event.EventRecorder
}
