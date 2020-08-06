package util

import (
	"fmt"

	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/common"
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

// GetSDKClient gets client
func GetSDKClient(sdkClient interface{}, exists bool) api.Client {
	if !exists {
		return nil
	}
	if sdkClient == nil {
		log.Instance.Warning("SdkClient could not be fetched")
		return nil
	}
	st, ok := sdkClient.(api.Client)
	if !ok {
		log.Instance.Warning("SdkClient could not be fetched")
		return nil
	}
	return st
}

// GetEventsClient gets client
func GetEventsClient(eventsClient interface{}, exists bool) api.Client {
	if !exists {
		return nil
	}
	if eventsClient == nil {
		log.Instance.Warning("EventsClient could not be fetched")
		return nil
	}
	st, ok := eventsClient.(api.Client)
	if !ok {
		log.Instance.Warning("EventsClient could not be fetched")
		return nil
	}
	return st
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
func AreValidAPIClient(httpClients common.HTTPClients) bool {
	if httpClients.EventsClient == nil {
		fmt.Println("EventsClient")
		return false
	}
	if httpClients.SdkClient == nil {
		fmt.Println("SdkClient")
		return false
	}
	return true
}

// AreValidStorages validates storages
func AreValidStorages(storages common.Storages) bool {
	if storages.SplitStorage == nil {
		fmt.Println("SplitStorage")
		return false
	}
	if storages.LocalTelemetryStorage == nil {
		fmt.Println("LocalTelemetryStorage")
		return false
	}
	if storages.SegmentStorage == nil {
		fmt.Println("SegmentStorage")
		return false
	}
	if appcontext.ExecutionMode() == appcontext.ProducerMode {

		if storages.EventStorage == nil {
			fmt.Println("EventStorage")
			return false
		}
		if storages.ImpressionStorage == nil {
			fmt.Println("ImpressionStorage")
			return false
		}
	}
	return true
}
