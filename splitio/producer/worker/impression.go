package worker

import (
	"strings"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/impression"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/common"
	"github.com/splitio/go-toolkit/logging"
)

const (
	postImpressionsLatencies = "testImpressions.time"
	postImpressionsCounters  = "testImpressions.status.{status}"
)

// RecorderImpressionMultiple struct for event sync
type RecorderImpressionMultiple struct {
	impressionStorage  storage.ImpressionStorageConsumer
	impressionRecorder service.ImpressionsRecorder
	metricStorage      storage.MetricsStorageProducer
	mutext             *sync.Mutex
	logger             logging.LoggerInterface
}

// NewImpressionRecordMultiple creates new event synchronizer for posting impressions
func NewImpressionRecordMultiple(
	impressionStorage storage.ImpressionStorageConsumer,
	impressionRecorder service.ImpressionsRecorder,
	metricStorage storage.MetricsStorageProducer,
	logger logging.LoggerInterface,
) impression.ImpressionRecorder {
	return &RecorderImpressionMultiple{
		impressionStorage:  impressionStorage,
		impressionRecorder: impressionRecorder,
		metricStorage:      metricStorage,
		mutext:             &sync.Mutex{},
		logger:             logger,
	}
}

func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (map[dtos.Metadata][]dtos.Impression, error) {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	storedImpressions, err := r.impressionStorage.PopNWithMetadata(bulkSize) //PopN has a mutex, so this function can be async without issues
	if err != nil {
		r.logger.Error("(Task) Post Events fails fetching events from storage", err.Error())
		return nil, err
	}

	// grouping the information by instanceID/instanceIP, and then by feature name
	// collectedData := make(map[dtos.Metadata]map[string][]dtos.ImpressionDTO)

	// grouping the information by instanceID/instanceIP
	collectedData := make(map[dtos.Metadata][]dtos.Impression)

	for _, stored := range storedImpressions {
		_, instanceExists := collectedData[stored.Metadata]
		if !instanceExists {
			collectedData[stored.Metadata] = make([]dtos.Impression, 0)
		}

		collectedData[stored.Metadata] = append(
			collectedData[stored.Metadata],
			stored.Impression,
		)
	}

	/*
		for _, stored := range impressionsToSend {
			_, instanceExists := collectedData[stored.Metadata]
			if !instanceExists {
				collectedData[stored.Metadata] = make(map[string][]dtos.ImpressionDTO)
			}

			_, featureExists := collectedData[stored.Metadata][stored.Impression.FeatureName]
			if !featureExists {
				collectedData[stored.Metadata][stored.Impression.FeatureName] = make([]dtos.ImpressionDTO, 0)
			}

			collectedData[stored.Metadata][stored.Impression.FeatureName] = append(
				collectedData[stored.Metadata][stored.Impression.FeatureName],
				dtos.ImpressionDTO{
					BucketingKey: stored.Impression.BucketingKey,
					ChangeNumber: stored.Impression.ChangeNumber,
					KeyName:      stored.Impression.KeyName,
					Label:        stored.Impression.Label,
					Time:         stored.Impression.Time,
					Treatment:    stored.Impression.Treatment,
				},
			)
		}
	*/

	return collectedData, nil
}

// SynchronizeImpressions syncs impressions
func (r *RecorderImpressionMultiple) SynchronizeImpressions(bulkSize int64) error {
	impressionsToSend, err := r.fetch(bulkSize)
	if err != nil {
		return err
	}

	for metadata, impressions := range impressionsToSend {
		before := time.Now()
		err := common.WithAttempts(3, func() error {
			err := r.impressionRecorder.Record(impressions, metadata)
			if err != nil {
				r.logger.Error("Error posting impressions")
			}

			return nil
		})
		if err != nil {
			if _, ok := err.(*dtos.HTTPError); ok {
				r.metricStorage.IncCounter(strings.Replace(postImpressionsCounters, "{status}", string(err.(*dtos.HTTPError).Code), 1))
			}
			return err
		}
		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		r.metricStorage.IncLatency(postImpressionsLatencies, bucket)
		r.metricStorage.IncCounter(strings.Replace(postImpressionsCounters, "{status}", "200", 1))
	}
	return nil
}

// FlushImpressions flushes impressions
func (r *RecorderImpressionMultiple) FlushImpressions(bulkSize int64) error {
	return nil
}
