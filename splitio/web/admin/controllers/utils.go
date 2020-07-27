package controllers

import (
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/common"
)

func getImpressionStorage(impressionStorage interface{}, exists bool) storage.ImpressionStorage {
	if !exists {
		return nil
	}
	if impressionStorage == nil {
		log.Warning.Println("ImpressionStorage could not be fetched")
		return nil
	}
	st, ok := impressionStorage.(storage.ImpressionStorage)
	if !ok {
		log.Warning.Println("ImpressionStorage could not be fetched")
		return nil
	}
	return st
}

func getEventStorage(eventStorage interface{}, exists bool) storage.EventsStorage {
	if !exists {
		return nil
	}
	if eventStorage == nil {
		log.Warning.Println("EventStorage could not be fetched")
		return nil
	}
	st, ok := eventStorage.(storage.EventsStorage)
	if !ok {
		log.Warning.Println("EventStorage could not be fetched")
		return nil
	}
	return st
}

func getSplitStorage(splitStorage interface{}, exists bool) storage.SplitStorage {
	if !exists {
		return nil
	}
	if splitStorage == nil {
		log.Warning.Println("SplitStorage could not be fetched")
		return nil
	}
	st, ok := splitStorage.(storage.SplitStorage)
	if !ok {
		log.Warning.Println("SplitStorage could not be fetched")
		return nil
	}
	return st
}

func getSegmentStorage(segmentStorage interface{}, exists bool) storage.SegmentStorage {
	if !exists {
		return nil
	}
	if segmentStorage == nil {
		log.Warning.Println("SegmentStorage could not be fetched")
		return nil
	}
	st, ok := segmentStorage.(storage.SegmentStorage)
	if !ok {
		log.Warning.Println("SegmentStorage could not be fetched")
		return nil
	}
	return st
}

func getTelemetryStorage(metricStorage interface{}, exists bool) storage.MetricsStorage {
	if !exists {
		return nil
	}
	if metricStorage == nil {
		log.Warning.Println("MetricsStorage could not be fetched")
		return nil
	}
	st, ok := metricStorage.(storage.MetricsStorage)
	if !ok {
		log.Warning.Println("MetricsStorage could not be fetched")
		return nil
	}
	return st
}

func getSdkClient(sdkClient interface{}, exists bool) api.Client {
	if !exists {
		return nil
	}
	if sdkClient == nil {
		log.Warning.Println("SdkClient could not be fetched")
		return nil
	}
	st, ok := sdkClient.(api.Client)
	if !ok {
		log.Warning.Println("SdkClient could not be fetched")
		return nil
	}
	return st
}

func getEvenntsClient(eventsClient interface{}, exists bool) api.Client {
	if !exists {
		return nil
	}
	if eventsClient == nil {
		log.Warning.Println("EventsClient could not be fetched")
		return nil
	}
	st, ok := eventsClient.(api.Client)
	if !ok {
		log.Warning.Println("EventsClient could not be fetched")
		return nil
	}
	return st
}

func getRecorders(recorders interface{}, exists bool) *common.Recorders {
	if !exists {
		return nil
	}
	if recorders == nil {
		log.Warning.Println("Recorders could not be fetched")
		return nil
	}
	st, ok := recorders.(common.Recorders)
	if !ok {
		log.Warning.Println("Recorders could not be fetched")
		return nil
	}
	return &st
}

func areValidStorages(storages common.Storages) bool {
	if storages.SplitStorage == nil {
		return false
	}
	if storages.SegmentStorage == nil {
		return false
	}
	if storages.EventStorage == nil {
		return false
	}
	if storages.ImpressionStorage == nil {
		return false
	}
	if storages.LocalTelemetryStorage == nil {
		return false
	}
	if storages.TelemetryStorage == nil {
		return false
	}
	return true
}
