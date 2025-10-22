package worker

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/producer/storage"
	storageMocks "github.com/splitio/split-synchronizer/v5/splitio/producer/storage/mocks"

	"github.com/splitio/go-split-commons/v8/dtos"
	serviceMocks "github.com/splitio/go-split-commons/v8/service/mocks"
	"github.com/splitio/go-split-commons/v8/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"
)

func makeBucket(index int, count int64) []int64 {
	toRet := make([]int64, telemetry.LatencyBucketCount)
	toRet[index] = count
	return toRet
}

func TestTelemetryMultiWorker(t *testing.T) {

	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "m1"}
	metadata2 := dtos.Metadata{SDKVersion: "go-2.2.2", MachineIP: "5.6.7.8", MachineName: "m2"}

	store := storageMocks.RedisTelemetryConsumerMultiMock{
		PopLatenciesCall: func() storage.MultiMethodLatencies {
			return map[dtos.Metadata]dtos.MethodLatencies{
				metadata1: dtos.MethodLatencies{Treatment: makeBucket(1, 1), TreatmentsByFlagSet: makeBucket(1, 2), TreatmentsWithConfigByFlagSet: makeBucket(1, 3)},
				metadata2: dtos.MethodLatencies{Treatment: makeBucket(2, 1), TreatmentsByFlagSets: makeBucket(1, 3), TreatmentsWithConfigByFlagSets: makeBucket(1, 1)},
			}
		},
		PopExceptionsCall: func() storage.MultiMethodExceptions {
			return map[dtos.Metadata]dtos.MethodExceptions{
				metadata1: dtos.MethodExceptions{Treatment: 1, TreatmentsByFlagSet: 9, TreatmentsWithConfigByFlagSet: 12},
				metadata2: dtos.MethodExceptions{Treatment: 2, TreatmentsByFlagSets: 5, TreatmentsWithConfigByFlagSets: 13},
			}
		},
		PopConfigsCall: func() storage.MultiConfigs {
			return map[dtos.Metadata]dtos.Config{
				metadata1: dtos.Config{OperationMode: 1},
				metadata2: dtos.Config{OperationMode: 2},
			}
		},
	}

	configCalls := 0
	statsCalls := 0
	sync := serviceMocks.MockTelemetryRecorder{
		RecordConfigCall: func(config dtos.Config, metadata dtos.Metadata) error {
			configCalls++
			if metadata == metadata1 && config.OperationMode != 1 {
				t.Error("invalid oepration mode")
			}
			if metadata == metadata2 && config.OperationMode != 2 {
				t.Error("invalid oepration mode")
			}
			return nil
		},
		RecordStatsCall: func(stats dtos.Stats, metadata dtos.Metadata) error {
			statsCalls++
			if metadata == metadata1 {
				if l := stats.MethodLatencies.Treatment[1]; l != 1 {
					t.Error("invalid latency", l)
				}
				if l := stats.MethodLatencies.TreatmentsByFlagSet[1]; l != 2 {
					t.Error("invalid latency", l)
				}
				if l := stats.MethodLatencies.TreatmentsWithConfigByFlagSet[1]; l != 3 {
					t.Error("invalid latency", l)
				}
				if stats.MethodExceptions.Treatment != 1 {
					t.Error("invalid exception count")
				}
				if stats.MethodExceptions.TreatmentsByFlagSet != 9 {
					t.Error("invalid exception count")
				}
				if stats.MethodExceptions.TreatmentsWithConfigByFlagSet != 12 {
					t.Error("invalid exception count")
				}
			} else if metadata == metadata2 {
				if l := stats.MethodLatencies.Treatment[2]; l != 1 {
					t.Error("invalid latency", l)
				}
				if l := stats.MethodLatencies.TreatmentsByFlagSets[1]; l != 3 {
					t.Error("invalid latency", l)
				}
				if l := stats.MethodLatencies.TreatmentsWithConfigByFlagSets[1]; l != 1 {
					t.Error("invalid latency", l)
				}
				if stats.MethodExceptions.Treatment != 2 {
					t.Error("invalid exception count")
				}
				if stats.MethodExceptions.TreatmentsByFlagSets != 5 {
					t.Error("invalid exception count")
				}
				if stats.MethodExceptions.TreatmentsWithConfigByFlagSets != 13 {
					t.Error("invalid exception count")
				}
			}
			return nil
		},
	}

	worker := NewTelemetryMultiWorker(logger, &store, &sync)
	err := worker.SynchronizeStats()
	if err != nil {
		t.Error("no errors should have been returned.")
	}

	err = worker.SyncrhonizeConfigs()
	if err != nil {
		t.Error("no errors should have been returned.")
	}

	if configCalls != 2 || statsCalls != 2 {
		t.Error("invalid number of calls: ", configCalls, statsCalls)
	}
}
