package worker

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/impression"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/common"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/task"
)

// RecorderImpressionMultiple struct for impression sync
type RecorderImpressionMultiple struct {
	impressionStorage         storage.ImpressionStorageConsumer
	impressionRecorder        service.ImpressionsRecorder
	metricsWrapper            *storage.MetricWrapper
	impressionListenerEnabled bool
	mutext                    *sync.Mutex
	logger                    logging.LoggerInterface
}

// NewImpressionRecordMultiple creates new impression synchronizer for posting impressions
func NewImpressionRecordMultiple(
	impressionStorage storage.ImpressionStorageConsumer,
	impressionRecorder service.ImpressionsRecorder,
	metricsWrapper *storage.MetricWrapper,
	impressionListenerEnabled bool,
	logger logging.LoggerInterface,
) impression.ImpressionRecorder {
	return &RecorderImpressionMultiple{
		impressionStorage:         impressionStorage,
		impressionRecorder:        impressionRecorder,
		metricsWrapper:            metricsWrapper,
		impressionListenerEnabled: impressionListenerEnabled,
		mutext:                    &sync.Mutex{},
		logger:                    logger,
	}
}

func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (map[dtos.Metadata][]dtos.Impression, error) {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	storedImpressions, err := r.impressionStorage.PopNWithMetadata(bulkSize) // PopN has a mutex, so this function can be async without issues
	if err != nil {
		r.logger.Error("(Task) Post Impressions fails fetching impressions from storage", err.Error())
		return nil, err
	}

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

	return collectedData, nil
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
			// r.logger.Info(fmt.Sprintf("Impressions: %v", impressions))
			r.logger.Info("impressionsToSend: ", len(impressions))
			err := r.impressionRecorder.Record(impressions, metadata)
			if err != nil {
				r.logger.Error("Error posting impressions")
			}

			return nil
		})
		if err != nil {
			if _, ok := err.(*dtos.HTTPError); ok {
				r.metricsWrapper.StoreCounters(storage.TestImpressionsCounter, string(err.(*dtos.HTTPError).Code))
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
