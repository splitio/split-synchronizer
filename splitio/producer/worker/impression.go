package worker

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/splitio/go-split-commons/v2/dtos"
	"github.com/splitio/go-split-commons/v2/provisional"
	"github.com/splitio/go-split-commons/v2/service"
	"github.com/splitio/go-split-commons/v2/storage"
	"github.com/splitio/go-split-commons/v2/synchronizer/worker/impression"
	"github.com/splitio/go-split-commons/v2/util"
	"github.com/splitio/go-toolkit/v3/common"
	"github.com/splitio/go-toolkit/v3/logging"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"golang.org/x/exp/errors/fmt"
)

const (
	impressionObserverCacheSize = 500000
)

// RecorderImpressionMultiple struct for impression sync
type RecorderImpressionMultiple struct {
	impressionStorage         storage.ImpressionStorageConsumer
	impressionRecorder        service.ImpressionsRecorder
	metricsWrapper            *storage.MetricWrapper
	impressionListenerEnabled bool
	logger                    logging.LoggerInterface
	impObserver               provisional.ImpressionObserver
}

// NewImpressionRecordMultiple creates new impression synchronizer for posting impressions
func NewImpressionRecordMultiple(
	impressionStorage storage.ImpressionStorageConsumer,
	impressionRecorder service.ImpressionsRecorder,
	metricsWrapper *storage.MetricWrapper,
	impressionListenerEnabled bool,
	logger logging.LoggerInterface,
) impression.ImpressionRecorder {
	impObserver, _ := provisional.NewImpressionObserver(impressionObserverCacheSize)
	return &RecorderImpressionMultiple{
		impressionStorage:         impressionStorage,
		impressionRecorder:        impressionRecorder,
		metricsWrapper:            metricsWrapper,
		impressionListenerEnabled: impressionListenerEnabled,
		logger:                    logger,
		impObserver:               impObserver,
	}
}

func toImpressionsDTO(impressionsMap map[string][]dtos.ImpressionDTO) ([]dtos.ImpressionsDTO, error) {
	if impressionsMap == nil {
		return nil, fmt.Errorf("Impressions map cannot be null")
	}

	toReturn := make([]dtos.ImpressionsDTO, 0)
	for feature, impressions := range impressionsMap {
		toReturn = append(toReturn, dtos.ImpressionsDTO{
			TestName:       feature,
			KeyImpressions: impressions,
		})
	}
	return toReturn, nil
}

func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (map[dtos.Metadata][]dtos.ImpressionsDTO, error) {
	storedImpressions, err := r.impressionStorage.PopNWithMetadata(bulkSize) // PopN has a mutex, so this function can be async without issues
	if err != nil {
		r.logger.Error("(Task) Post Impressions fails fetching impressions from storage", err.Error())
		return nil, err
	}

	// grouping the information by instanceID/instanceIP, and then by feature name
	collectedData := make(map[dtos.Metadata]map[string][]dtos.ImpressionDTO)

	for _, stored := range storedImpressions {
		_, instanceExists := collectedData[stored.Metadata]
		if !instanceExists {
			collectedData[stored.Metadata] = make(map[string][]dtos.ImpressionDTO)
		}

		_, featureExists := collectedData[stored.Metadata][stored.Impression.FeatureName]
		if !featureExists {
			collectedData[stored.Metadata][stored.Impression.FeatureName] = make([]dtos.ImpressionDTO, 0)
		}

		imp := dtos.ImpressionDTO{
			BucketingKey: stored.Impression.BucketingKey,
			ChangeNumber: stored.Impression.ChangeNumber,
			KeyName:      stored.Impression.KeyName,
			Label:        stored.Impression.Label,
			Time:         stored.Impression.Time,
			Treatment:    stored.Impression.Treatment,
		}
		imp.Pt, _ = r.impObserver.TestAndSet(
			stored.Impression.FeatureName,
			&imp,
		)
		collectedData[stored.Metadata][stored.Impression.FeatureName] = append(
			collectedData[stored.Metadata][stored.Impression.FeatureName],
			imp,
		)
	}

	toReturn := make(map[dtos.Metadata][]dtos.ImpressionsDTO)
	for metadata, impsForMetadata := range collectedData {
		toReturn[metadata], err = toImpressionsDTO(impsForMetadata)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Unable to write impressions for metadata %v", metadata))
			continue
		}
	}

	return toReturn, nil
}

func (r *RecorderImpressionMultiple) synchronizeImpressions(bulkSize int64) error {
	impressionsToSend, err := r.fetch(bulkSize)
	if err != nil {
		return err
	}

	for metadata, impressions := range impressionsToSend {
		before := time.Now()
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			task.StoreDataFlushed(before.UnixNano(), len(impressions), r.impressionStorage.Count(), "impressions")
		}
		err := common.WithAttempts(3, func() error {
			r.logger.Info("impressionsToSend: ", len(impressions))
			err := r.impressionRecorder.Record(impressions, metadata)
			if err != nil {
				r.logger.Error("Error posting impressions")
			}

			return nil
		})
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				r.metricsWrapper.StoreCounters(storage.TestImpressionsCounter, string(httpError.Code))
			}
			return err
		}
		if r.impressionListenerEnabled {
			rawImpressions, err := json.Marshal(impressions)
			if err != nil {
				r.logger.Error("JSON encoding failed for the following impressions", impressions)
				continue
			}
			err = task.QueueImpressionsForListener(&task.ImpressionBulk{
				Data:        json.RawMessage(rawImpressions),
				SdkVersion:  metadata.SDKVersion,
				MachineIP:   metadata.MachineIP,
				MachineName: metadata.MachineName,
			})
			if err != nil {
				log.Instance.Error(err)
			}
		}
		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		r.metricsWrapper.StoreLatencies(storage.TestImpressionsLatency, bucket)
		r.metricsWrapper.StoreCounters(storage.TestImpressionsCounter, "ok")
	}
	return nil
}

// SynchronizeImpressions syncs impressions
func (r *RecorderImpressionMultiple) SynchronizeImpressions(bulkSize int64) error {
	if task.IsOperationRunning(task.ImpressionsOperation) {
		r.logger.Debug("Another task executed by the user is performing operations on Impressions. Skipping.")
		return nil
	}

	return r.synchronizeImpressions(bulkSize)
}

// FlushImpressions flushes impressions
func (r *RecorderImpressionMultiple) FlushImpressions(bulkSize int64) error {
	if task.RequestOperation(task.ImpressionsOperation) {
		defer task.FinishOperation(task.ImpressionsOperation)
	} else {
		r.logger.Debug("Cannot execute flush. Another operation is performing operations on Impressions.")
		return errors.New("Cannot execute flush. Another operation is performing operations on Impressions")
	}
	elementsToFlush := splitio.MaxSizeToFlush

	if bulkSize != 0 {
		elementsToFlush = bulkSize
	}

	for elementsToFlush > 0 && r.impressionStorage.Count() > 0 {
		maxSize := splitio.DefaultSize
		if elementsToFlush < splitio.DefaultSize {
			maxSize = elementsToFlush
		}
		err := r.synchronizeImpressions(maxSize)
		if err != nil {
			return err
		}
		elementsToFlush = elementsToFlush - splitio.DefaultSize
	}
	return nil
}
