package util

import (
	"github.com/splitio/go-split-commons/v3/storage"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
)

// GetImpressionStorage gets storage
func GetImpressionStorage(impressionStorage interface{}, exists bool) storage.ImpressionStorage {
	if !exists {
		return nil
	}
	if impressionStorage == nil {
		log.Instance.Warning("ImpressionStorage could not be fetched")
		return nil
	}
	st, ok := impressionStorage.(storage.ImpressionStorage)
	if !ok {
		log.Instance.Warning("ImpressionStorage could not be fetched")
		return nil
	}
	return st
}

// GetEventStorage gets storage
func GetEventStorage(eventStorage interface{}, exists bool) storage.EventsStorage {
	if !exists {
		return nil
	}
	if eventStorage == nil {
		log.Instance.Warning("EventStorage could not be fetched")
		return nil
	}
	st, ok := eventStorage.(storage.EventsStorage)
	if !ok {
		log.Instance.Warning("EventStorage could not be fetched")
		return nil
	}
	return st
}

// GetSplitStorage gets storage
func GetSplitStorage(splitStorage interface{}, exists bool) storage.SplitStorage {
	if !exists {
		return nil
	}
	if splitStorage == nil {
		log.Instance.Warning("SplitStorage could not be fetched")
		return nil
	}
	st, ok := splitStorage.(storage.SplitStorage)
	if !ok {
		log.Instance.Warning("SplitStorage could not be fetched")
		return nil
	}
	return st
}

// GetSegmentStorage gets storage
func GetSegmentStorage(segmentStorage interface{}, exists bool) storage.SegmentStorage {
	if !exists {
		return nil
	}
	if segmentStorage == nil {
		log.Instance.Warning("SegmentStorage could not be fetched")
		return nil
	}
	st, ok := segmentStorage.(storage.SegmentStorage)
	if !ok {
		log.Instance.Warning("SegmentStorage could not be fetched")
		return nil
	}
	return st
}

// GetTelemetryStorage gets storage
func GetTelemetryStorage(metricStorage interface{}, exists bool) storage.MetricsStorage {
	if !exists {
		return nil
	}
	if metricStorage == nil {
		log.Instance.Warning("MetricsStorage could not be fetched")
		return nil
	}
	st, ok := metricStorage.(storage.MetricsStorage)
	if !ok {
		log.Instance.Warning("MetricsStorage could not be fetched")
		return nil
	}
	return st
}

// GetHTTPClients gets client
func GetHTTPClients(httpClients interface{}, exists bool) *common.HTTPClients {
	if !exists {
		return nil
	}
	if httpClients == nil {
		log.Instance.Warning("HTTPClients could not be fetched")
		return nil
	}
	st, ok := httpClients.(common.HTTPClients)
	if !ok {
		log.Instance.Warning("HTTPClients could not be fetched")
		return nil
	}
	return &st
}

// GetRecorders gets recorders
func GetRecorders(recorders interface{}, exists bool) *common.Recorders {
	if !exists {
		return nil
	}
	if recorders == nil {
		log.Instance.Warning("Recorders could not be fetched")
		return nil
	}
	st, ok := recorders.(common.Recorders)
	if !ok {
		log.Instance.Warning("Recorders could not be fetched")
		return nil
	}
	return &st
}

// AreValidAPIClient validates http clients
func AreValidAPIClient(httpClients *common.HTTPClients) bool {
	if httpClients == nil {
		return false
	}
	if httpClients.EventsClient == nil {
		return false
	}
	if httpClients.SdkClient == nil {
		return false
	}
	if httpClients.AuthClient == nil {
		return false
	}
	return true
}

// AreValidStorages validates storages
func AreValidStorages(storages common.Storages) bool {
	if storages.SplitStorage == nil {
		return false
	}
	if storages.LocalTelemetryStorage == nil {
		return false
	}
	if storages.SegmentStorage == nil {
		return false
	}
	if appcontext.ExecutionMode() == appcontext.ProducerMode {

		if storages.EventStorage == nil {
			return false
		}
		if storages.ImpressionStorage == nil {
			return false
		}
	}
	return true
}
