package tasks

import (
	"fmt"
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/assert"
)

func TestWorkerFactoriesGenerateSequentialNames(t *testing.T) {
	logger := logging.NewLogger(nil)

	factories := map[string]WorkerFactory{
		"impressions-worker":                newImpressionWorkerFactory("impressions-worker", nil, logger),
		"impressions-count-worker":          newImpressionCountWorkerFactory("impressions-count-worker", nil, logger),
		"events-worker":                     newEventWorkerFactory("events-worker", nil, logger),
		"telemetry-config-worker":           newTelemetryConfigWorkerFactory("telemetry-config-worker", nil, logger),
		"telemetry-usage-worker":            newTelemetryUsageWorkerFactory("telemetry-usage-worker", nil, logger),
		"telemetry-keys-client-side-worker": newTelemetryKeysClientSideWorkerFactory("telemetry-keys-client-side-worker", nil, logger),
		"telemetry-keys-server-side-worker": newTelemetryKeysServerSideWorkerFactory("telemetry-keys-server-side-worker", nil, logger),
	}

	for prefix, factory := range factories {
		t.Run(prefix, func(t *testing.T) {
			for i := 0; i < 3; i++ {
				assert.Equal(t, fmt.Sprintf("%s_%d", prefix, i), factory().Name())
			}
		})
	}
}

func TestWorkerFactoriesHaveIndependentCounters(t *testing.T) {
	logger := logging.NewLogger(nil)
	f1 := newImpressionWorkerFactory("impressions-worker", nil, logger)
	f2 := newImpressionWorkerFactory("impressions-worker", nil, logger)

	assert.Equal(t, "impressions-worker_0", f1().Name())
	assert.Equal(t, "impressions-worker_1", f1().Name())
	assert.Equal(t, "impressions-worker_0", f2().Name())
}

func TestDeferredRecordingTaskRegistersAllWorkers(t *testing.T) {
	logger := logging.NewLogger(nil)
	const threads = 3

	tests := []struct {
		prefix string
		task   *DeferredRecordingTaskImpl
	}{
		{"impressions-worker", NewImpressionsFlushTask(nil, logger, 3600, 10, threads)},
		{"impressions-count-worker", NewImpressionCountFlushTask(nil, logger, 3600, 10, threads)},
		{"events-worker", NewEventsFlushTask(nil, logger, 3600, 10, threads)},
		{"telemetry-config-worker", NewTelemetryConfigFlushTask(nil, logger, 3600, 10, threads)},
		{"telemetry-usage-worker", NewTelemetryUsageFlushTask(nil, logger, 3600, 10, threads)},
		{"telemetry-keys-client-side-worker", NewTelemetryKeysClientSideFlushTask(nil, logger, 3600, 10, threads)},
		{"telemetry-keys-server-side-worker", NewTelemetryKeysServerSideFlushTask(nil, logger, 3600, 10, threads)},
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			names := []string{tc.prefix + "_0", tc.prefix + "_1", tc.prefix + "_2"}

			// workers are started asynchronously. every one of them must end up registered under its own name
			assert.Eventually(t, func() bool {
				for _, name := range names {
					if !tc.task.pool.IsWorkerRunning(name) {
						return false
					}
				}
				return true
			}, 2*time.Second, 5*time.Millisecond)

			// DeferredRecordingTaskImpl.Stop only stops the flushing task, so the pool is stopped directly
			assert.Nil(t, tc.task.pool.StopAll(true))
			for _, name := range names {
				assert.False(t, tc.task.pool.IsWorkerRunning(name), name)
			}
		})
	}
}
