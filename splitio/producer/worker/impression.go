package worker

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/splitio/go-split-commons/conf"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/provisional"
	"github.com/splitio/go-split-commons/service"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/synchronizer/worker/impression"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/go-toolkit/common"
	"github.com/splitio/go-toolkit/logging"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/task"
	"golang.org/x/exp/errors/fmt"
)

// RecorderImpressionMultiple struct for impression sync
type RecorderImpressionMultiple struct {
	impressionStorage         storage.ImpressionStorageConsumer
	impressionRecorder        service.ImpressionsRecorder
	metricsWrapper            *storage.MetricWrapper
	impressionListenerEnabled bool
	logger                    logging.LoggerInterface
	impressionManager         provisional.ImpressionManager
	mode                      string
}

// NewImpressionRecordMultiple creates new impression synchronizer for posting impressions
func NewImpressionRecordMultiple(
	impressionStorage storage.ImpressionStorageConsumer,
	impressionRecorder service.ImpressionsRecorder,
	metricsWrapper *storage.MetricWrapper,
	logger logging.LoggerInterface,
	managerConfig conf.ManagerConfig,
	impressionsCounter *provisional.ImpressionsCounter,
) (impression.ImpressionRecorder, error) {
	impressionManager, err := provisional.NewImpressionManager(managerConfig, impressionsCounter)
	if err != nil {
		return nil, err
	}
	return &RecorderImpressionMultiple{
		impressionStorage:         impressionStorage,
		impressionRecorder:        impressionRecorder,
		metricsWrapper:            metricsWrapper,
		impressionListenerEnabled: managerConfig.ListenerEnabled,
		logger:                    logger,
		impressionManager:         impressionManager,
		mode:                      managerConfig.ImpressionsMode,
	}, nil
}

func (r *RecorderImpressionMultiple) wrapDTO(collectedData map[dtos.Metadata]map[string][]dtos.ImpressionDTO) map[dtos.Metadata][]dtos.ImpressionsDTO {
	var err error
	impressions := make(map[dtos.Metadata][]dtos.ImpressionsDTO)
	for metadata, impsForMetadata := range collectedData {
		impressions[metadata], err = toImpressionsDTO(impsForMetadata)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Unable to write impressions for metadata %v", metadata))
			continue
		}
	}
	return impressions
}

func (r *RecorderImpressionMultiple) fetch(bulkSize int64) (map[dtos.Metadata][]dtos.ImpressionsDTO, map[dtos.Metadata][]impressionsListener, error) {
	storedImpressions, err := r.impressionStorage.PopNWithMetadata(bulkSize) // PopN has a mutex, so this function can be async without issues
	if err != nil {
		r.logger.Error("(Task) Post Impressions fails fetching impressions from storage", err.Error())
		return nil, nil, err
	}

	// grouping the information by instanceID/instanceIP, and then by feature name
	collectedDataforLog := make(map[dtos.Metadata]map[string][]dtos.ImpressionDTO)
	collectedDataforListener := make(map[dtos.Metadata]map[string][]impressionListener)

	for _, stored := range storedImpressions {
		toSend, forListener := r.impressionManager.ProcessImpressions([]dtos.Impression{stored.Impression})

		collectedDataforLog = wrapData(toSend, collectedDataforLog, stored.Metadata)
		collectedDataforListener = wrapDataForListener(forListener, collectedDataforListener, stored.Metadata)
	}

	return r.wrapDTO(collectedDataforLog), wrapDTOListener(collectedDataforListener), nil
}

func (r *RecorderImpressionMultiple) recordImpressions(impressionsToSend map[dtos.Metadata][]dtos.ImpressionsDTO) error {
	for metadata, impressions := range impressionsToSend {
		before := time.Now()
		if appcontext.ExecutionMode() == appcontext.ProducerMode {
			task.StoreDataFlushed(before.UnixNano(), len(impressions), r.impressionStorage.Count(), "impressions")
		}
		err := common.WithAttempts(3, func() error {
			r.logger.Debug("impressionsToSend: ", len(impressions))
			err := r.impressionRecorder.Record(impressions, metadata, map[string]string{"SplitSDKImpressionsMode": r.mode})
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
		bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
		r.metricsWrapper.StoreLatencies(storage.TestImpressionsLatency, bucket)
		r.metricsWrapper.StoreCounters(storage.TestImpressionsCounter, "ok")
	}
	return nil
}

func (r *RecorderImpressionMultiple) sendDataToListener(impressionsToListener map[dtos.Metadata][]impressionsListener) {
	for metadata, impressions := range impressionsToListener {
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
			r.logger.Error(err)
		}
	}
}

func (r *RecorderImpressionMultiple) synchronizeImpressions(bulkSize int64) error {
	impressionsToSend, impressionsForListener, err := r.fetch(bulkSize)
	if err != nil {
		return err
	}

	err = r.recordImpressions(impressionsToSend)
	if err != nil {
		return err
	}
	if r.impressionListenerEnabled {
		r.sendDataToListener(impressionsForListener)
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
