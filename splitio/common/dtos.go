package common

import (
	"github.com/splitio/go-split-commons/v2/service/api"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-split-commons/v2/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/v2/synchronizer/worker/impression"
)

// Storages wraps storages in one struct
type Storages struct {
	SplitStorage          storage.SplitStorage
	SegmentStorage        storage.SegmentStorage
	LocalTelemetryStorage storage.MetricsStorage
	EventStorage          storage.EventsStorage
	ImpressionStorage     storage.ImpressionStorage
}

// HTTPClients wraps http clients for healthcheck
type HTTPClients struct {
	AuthClient   api.Client
	SdkClient    api.Client
	EventsClient api.Client
}

// Recorders wraps recorders for dashboards
type Recorders struct {
	Impression impression.ImpressionRecorder
	Event      event.EventRecorder
}
