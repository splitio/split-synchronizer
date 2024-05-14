package worker

import (
	"github.com/splitio/go-split-commons/v6/provisional/strategy"
	"github.com/splitio/go-split-commons/v6/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

type ImpressionsCounstWorkerImp struct {
	impressionsCounter strategy.ImpressionsCounter
	storage            storage.ImpressionsCountConsumer
	logger             logging.LoggerInterface
}

func NewImpressionsCounstWorker(
	impressionsCounter strategy.ImpressionsCounter,
	storage storage.ImpressionsCountConsumer,
	logger logging.LoggerInterface,
) ImpressionsCounstWorkerImp {
	return ImpressionsCounstWorkerImp{
		impressionsCounter: impressionsCounter,
		storage:            storage,
		logger:             logger,
	}
}

func (i *ImpressionsCounstWorkerImp) Process() error {
	impcounts, err := i.storage.GetImpressionsCount()
	if err != nil {
		return err
	}

	for _, count := range impcounts.PerFeature {
		i.impressionsCounter.Inc(count.FeatureName, count.TimeFrame, count.RawCount)
	}

	return nil
}
