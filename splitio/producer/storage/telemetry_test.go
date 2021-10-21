package storage

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
	redisSt "github.com/splitio/go-split-commons/v4/storage/redis"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"
)

// TODO(mredolatti): Move this somewhere into toolkit
// \{
var ErrNoCaller = errors.New("no caller")
var ErrNilCaller = errors.New("nil caller")

func getCurrentFuncName() (string, error) {
	fpcs := make([]uintptr, 1)

	// Skip 2 levels to get the caller
	n := runtime.Callers(2, fpcs)
	if n == 0 {
		return "", ErrNoCaller
	}

	caller := runtime.FuncForPC(fpcs[0] - 1)
	if caller == nil {
		return "", ErrNilCaller
	}

	return caller.Name(), nil

}

// \}

func TestRedisTelemetryExceptions(t *testing.T) {
	redisPrefix, _ := getCurrentFuncName()
	innerClient, _ := redis.NewClient(&redis.UniversalOptions{})
	client, _ := redis.NewPrefixedRedisClient(innerClient, redisPrefix)
	defer func() {
		keys, _ := innerClient.Keys(redisPrefix + "*").Multi()
		innerClient.Del(keys...)
	}()

	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "m1"}
	metadata2 := dtos.Metadata{SDKVersion: "go-2.2.2", MachineIP: "5.6.7.8", MachineName: "m2"}

	producer1 := redisSt.NewTelemetryStorage(client, logger, metadata1)
	producer2 := redisSt.NewTelemetryStorage(client, logger, metadata2)

	producer1.RecordException(telemetry.Treatment)
	producer1.RecordException(telemetry.Treatments)
	producer1.RecordException(telemetry.Treatments)
	producer1.RecordException(telemetry.TreatmentWithConfig)
	producer1.RecordException(telemetry.TreatmentWithConfig)
	producer1.RecordException(telemetry.TreatmentWithConfig)
	producer1.RecordException(telemetry.TreatmentsWithConfig)
	producer1.RecordException(telemetry.TreatmentsWithConfig)
	producer1.RecordException(telemetry.TreatmentsWithConfig)
	producer1.RecordException(telemetry.TreatmentsWithConfig)
	producer1.RecordException(telemetry.Track)
	producer1.RecordException(telemetry.Track)
	producer1.RecordException(telemetry.Track)
	producer1.RecordException(telemetry.Track)
	producer1.RecordException(telemetry.Track)

	producer2.RecordException(telemetry.Treatment)
	producer2.RecordException(telemetry.Treatment)
	producer2.RecordException(telemetry.Treatment)
	producer2.RecordException(telemetry.Treatment)
	producer2.RecordException(telemetry.Treatment)
	producer2.RecordException(telemetry.Treatments)
	producer2.RecordException(telemetry.Treatments)
	producer2.RecordException(telemetry.Treatments)
	producer2.RecordException(telemetry.Treatments)
	producer2.RecordException(telemetry.TreatmentWithConfig)
	producer2.RecordException(telemetry.TreatmentWithConfig)
	producer2.RecordException(telemetry.TreatmentWithConfig)
	producer2.RecordException(telemetry.TreatmentsWithConfig)
	producer2.RecordException(telemetry.TreatmentsWithConfig)
	producer2.RecordException(telemetry.Track)

	consumer := NewRedisTelemetryCosumerclient(client, logger)
	exceptions := consumer.PopExceptions()

	if len(exceptions) != 2 {
		t.Error("should have 2 different metadatas")
	}

	excsForM1, ok := exceptions[metadata1]
	if !ok {
		t.Error("exceptions for metadata1 should be present")
	}

	if excsForM1.Treatment != 1 {
		t.Error("exception count for track in metadata1 should be 1. Was: ", excsForM1.Treatment)
	}

	if excsForM1.Treatments != 2 {
		t.Error("exception count for track in metadata1 should be 2. Was: ", excsForM1.Treatments)
	}

	if excsForM1.TreatmentWithConfig != 3 {
		t.Error("exception count for track in metadata1 should be 3. Was: ", excsForM1.TreatmentWithConfig)
	}

	if excsForM1.TreatmentsWithConfig != 4 {
		t.Error("exception count for track in metadata1 should be 4. Was: ", excsForM1.TreatmentsWithConfig)
	}

	if excsForM1.Track != 5 {
		t.Error("exception count for track in metadata1 should be 5. Was: ", excsForM1.Track)
	}

	excsForM2, ok := exceptions[metadata2]
	if !ok {
		t.Error("exceptions for metadata2 should be present")
	}

	if excsForM2.Treatment != 5 {
		t.Error("exception count for track in metadata1 should be 5. Was: ", excsForM2.Treatment)
	}

	if excsForM2.Treatments != 4 {
		t.Error("exception count for track in metadata1 should be 4. Was: ", excsForM2.Treatments)
	}

	if excsForM2.TreatmentWithConfig != 3 {
		t.Error("exception count for track in metadata1 should be 3. Was: ", excsForM2.TreatmentWithConfig)
	}

	if excsForM2.TreatmentsWithConfig != 2 {
		t.Error("exception count for track in metadata1 should be 2. Was: ", excsForM2.TreatmentsWithConfig)
	}

	if excsForM2.Track != 1 {
		t.Error("exception count for track in metadata1 should be 1. Was: ", excsForM2.Track)
	}

	exceptions = consumer.PopExceptions()
	if len(exceptions) > 0 {
		t.Error("no more exceptions should have been fetched from redis. Got:", exceptions)
	}
}

func TestRedisTelemetryLatencies(t *testing.T) {
	redisPrefix, _ := getCurrentFuncName()
	innerClient, _ := redis.NewClient(&redis.UniversalOptions{})
	client, _ := redis.NewPrefixedRedisClient(innerClient, redisPrefix)
	defer func() {
		keys, _ := innerClient.Keys(redisPrefix + "*").Multi()
		innerClient.Del(keys...)
	}()

	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "m1"}
	metadata2 := dtos.Metadata{SDKVersion: "go-2.2.2", MachineIP: "5.6.7.8", MachineName: "m2"}

	producer1 := redisSt.NewTelemetryStorage(client, logger, metadata1)
	producer2 := redisSt.NewTelemetryStorage(client, logger, metadata2)

	producer1.RecordLatency(telemetry.Treatment, 1*time.Second)
	producer1.RecordLatency(telemetry.Treatments, 2*time.Second)
	producer1.RecordLatency(telemetry.TreatmentWithConfig, 3*time.Second)
	producer1.RecordLatency(telemetry.TreatmentsWithConfig, 4*time.Second)
	producer1.RecordLatency(telemetry.Track, 5*time.Second)

	producer2.RecordLatency(telemetry.Treatment, 5*time.Second)
	producer2.RecordLatency(telemetry.Treatments, 4*time.Second)
	producer2.RecordLatency(telemetry.TreatmentWithConfig, 3*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfig, 2*time.Second)
	producer2.RecordLatency(telemetry.Track, 1*time.Second)

	consumer := NewRedisTelemetryCosumerclient(client, logger)
	latencies := consumer.PopLatencies()

	if len(latencies) != 2 {
		t.Error("should have 2 different metadatas")
	}

	latsForM1, ok := latencies[metadata1]
	if !ok {
		t.Error("latencies for metadata1 should be present")
	}

	l1Treatment := int64(0)
	for _, count := range latsForM1.Treatment {
		l1Treatment += count
	}
	if l1Treatment != int64(1) {
		t.Error("latency count for .Treatment should be 1. Is: ", l1Treatment)
	}

	l1Treatments := int64(0)
	for _, count := range latsForM1.Treatments {
		l1Treatments += count
	}
	if l1Treatments != int64(1) {
		t.Error("latency count for .Treatments should be 1. Is: ", l1Treatments)
	}

	l1TreatmentWithConfig := int64(0)
	for _, count := range latsForM1.TreatmentWithConfig {
		l1TreatmentWithConfig += count
	}
	if l1TreatmentWithConfig != 1 {
		t.Error("latency count for .TreatmentWithConfig should be 1. Is: ", l1TreatmentWithConfig)
	}

	l1TreatmentsWithConfig := int64(0)
	for _, count := range latsForM1.TreatmentsWithConfig {
		l1TreatmentsWithConfig += count
	}
	if l1TreatmentsWithConfig != 1 {
		t.Error("latency count for .TreatmentsWithConfig should be 1. Is: ", l1TreatmentsWithConfig)
	}

	l1Track := int64(0)
	for _, count := range latsForM1.Track {
		l1Track += count
	}
	if l1Track != 1 {
		t.Error("latency count for .Track should be 1. Is: ", l1Track)
	}

	latsForM2, ok := latencies[metadata2]
	if !ok {
		t.Error("latencies for metadata2 should be present")
	}

	l2Treatment := int64(0)
	for _, count := range latsForM2.Treatment {
		l2Treatment += count
	}
	if l2Treatment != int64(1) {
		t.Error("latency count for .Treatment should be 1. Is: ", l2Treatment)
	}

	l2Treatments := int64(0)
	for _, count := range latsForM2.Treatments {
		l2Treatments += count
	}
	if l2Treatments != int64(1) {
		t.Error("latency count for .Treatments should be 1. Is: ", l2Treatments)
	}

	l2TreatmentWithConfig := int64(0)
	for _, count := range latsForM2.TreatmentWithConfig {
		l2TreatmentWithConfig += count
	}
	if l2TreatmentWithConfig != 1 {
		t.Error("latency count for .TreatmentWithConfig should be 1. Is: ", l2TreatmentWithConfig)
	}

	l2TreatmentsWithConfig := int64(0)
	for _, count := range latsForM2.TreatmentsWithConfig {
		l2TreatmentsWithConfig += count
	}
	if l2TreatmentsWithConfig != 1 {
		t.Error("latency count for .TreatmentsWithConfig should be 1. Is: ", l2TreatmentsWithConfig)
	}

	l2Track := int64(0)
	for _, count := range latsForM2.Track {
		l2Track += count
	}
	if l2Track != 1 {
		t.Error("latency count for .Track should be 1. Is: ", l2Track)
	}
}

func TestPopConfigs(t *testing.T) {
	redisPrefix, _ := getCurrentFuncName()
	innerClient, _ := redis.NewClient(&redis.UniversalOptions{})
	client, _ := redis.NewPrefixedRedisClient(innerClient, redisPrefix)
	defer func() {
		keys, _ := innerClient.Keys(redisPrefix + "*").Multi()
		innerClient.Del(keys...)
	}()

	logger := logging.NewLogger(nil)

	metadata1 := dtos.Metadata{SDKVersion: "go-1.1.1", MachineIP: "1.2.3.4", MachineName: "m1"}
	metadata2 := dtos.Metadata{SDKVersion: "go-2.2.2", MachineIP: "5.6.7.8", MachineName: "m2"}

	producer1 := redisSt.NewTelemetryStorage(client, logger, metadata1)
	producer2 := redisSt.NewTelemetryStorage(client, logger, metadata2)

	producer1.RecordConfigData(dtos.Config{OperationMode: 100})
	producer1.RecordConfigData(dtos.Config{OperationMode: 1})
	producer2.RecordConfigData(dtos.Config{OperationMode: 1})
	producer2.RecordConfigData(dtos.Config{OperationMode: 2})

	consumer := NewRedisTelemetryCosumerclient(client, logger)

	configs := consumer.PopConfigs()

	if len(configs) != 2 {
		t.Error("there should be 2 entries, one for each metadata")
	}

	forM1, ok := configs[metadata1]
	if !ok {
		t.Error("config for metadata1 should be present")
	}

	if forM1.OperationMode != 1 {
		t.Error("the last version should have been the one kept")
	}

	forM2, ok := configs[metadata2]
	if !ok {
		t.Error("config for metadata1 should be present")
	}

	if forM2.OperationMode != 2 {
		t.Error("the last version should have been the one kept")
	}
}
