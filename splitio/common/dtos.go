package common

import (
	"github.com/splitio/go-split-commons/v3/service/api"
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/event"
	"github.com/splitio/go-split-commons/v3/synchronizer/worker/impression"
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

// ImpressionListener struct for payload
type ImpressionListener struct {
	KeyName      string `json:"keyName"`
	Treatment    string `json:"treatment"`
	Time         int64  `json:"time"`
	ChangeNumber int64  `json:"changeNumber"`
	Label        string `json:"label"`
	BucketingKey string `json:"bucketingKey,omitempty"`
	Pt           int64  `json:"pt,omitempty"`
}

// ImpressionsListener struct for payload
type ImpressionsListener struct {
	TestName       string               `json:"testName"`
	KeyImpressions []ImpressionListener `json:"keyImpressions"`
}
