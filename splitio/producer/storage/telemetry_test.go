package storage

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v5/dtos"
	redisSt "github.com/splitio/go-split-commons/v5/storage/redis"
	"github.com/splitio/go-split-commons/v5/telemetry"
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
	producer1.RecordException(telemetry.TreatmentsByFlagSet)
	producer1.RecordException(telemetry.TreatmentsByFlagSet)
	producer1.RecordException(telemetry.TreatmentsByFlagSet)
	producer1.RecordException(telemetry.TreatmentsByFlagSet)
	producer1.RecordException(telemetry.TreatmentsByFlagSets)
	producer1.RecordException(telemetry.TreatmentsByFlagSets)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSet)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSet)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSet)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer1.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
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
	producer2.RecordException(telemetry.TreatmentsByFlagSet)
	producer2.RecordException(telemetry.TreatmentsByFlagSet)
	producer2.RecordException(telemetry.TreatmentsByFlagSet)
	producer2.RecordException(telemetry.TreatmentsByFlagSets)
	producer2.RecordException(telemetry.TreatmentsWithConfigByFlagSet)
	producer2.RecordException(telemetry.TreatmentsWithConfigByFlagSet)
	producer2.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer2.RecordException(telemetry.TreatmentsWithConfigByFlagSets)
	producer2.RecordException(telemetry.TreatmentsWithConfigByFlagSets)

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

	if excsForM1.TreatmentsByFlagSet != 4 {
		t.Error("exception count for treatmentsByFlagSet in metadata1 should be 4. Was: ", excsForM1.TreatmentsByFlagSet)
	}

	if excsForM1.TreatmentsByFlagSets != 2 {
		t.Error("exception count for treatmentsByFlagSets in metadata1 should be 2. Was: ", excsForM1.TreatmentsByFlagSets)
	}

	if excsForM1.TreatmentsWithConfigByFlagSet != 3 {
		t.Error("exception count for treatmentsWithConfigByFlagSet in metadata1 should be 3. Was: ", excsForM1.TreatmentsWithConfigByFlagSet)
	}

	if excsForM1.TreatmentsWithConfigByFlagSets != 5 {
		t.Error("exception count for treatmentsWithConfigByFlagSets in metadata1 should be 5. Was: ", excsForM1.TreatmentsWithConfigByFlagSets)
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

	if excsForM2.TreatmentsByFlagSet != 3 {
		t.Error("exception count for treatmentsByFlagSet in metadata2 should be 3. Was: ", excsForM2.TreatmentsByFlagSet)
	}

	if excsForM2.TreatmentsByFlagSets != 1 {
		t.Error("exception count for treatmentsByFlagSets in metadata2 should be 1. Was: ", excsForM2.TreatmentsByFlagSets)
	}

	if excsForM2.TreatmentsWithConfigByFlagSet != 2 {
		t.Error("exception count for treatmentsWithConfigByFlagSet in metadata2 should be 2. Was: ", excsForM2.TreatmentsWithConfigByFlagSet)
	}

	if excsForM2.TreatmentsWithConfigByFlagSets != 3 {
		t.Error("exception count for treatmentsWithConfigByFlagSets in metadata2 should be 3. Was: ", excsForM2.TreatmentsWithConfigByFlagSets)
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
	producer1.RecordLatency(telemetry.TreatmentsByFlagSet, 2*time.Second)
	producer1.RecordLatency(telemetry.TreatmentsByFlagSets, 4*time.Second)
	producer1.RecordLatency(telemetry.TreatmentsWithConfigByFlagSet, 3*time.Second)
	producer1.RecordLatency(telemetry.TreatmentsWithConfigByFlagSet, 6*time.Second)
	producer1.RecordLatency(telemetry.TreatmentsWithConfigByFlagSets, 5*time.Second)
	producer1.RecordLatency(telemetry.Track, 5*time.Second)

	producer2.RecordLatency(telemetry.Treatment, 5*time.Second)
	producer2.RecordLatency(telemetry.Treatments, 4*time.Second)
	producer2.RecordLatency(telemetry.TreatmentWithConfig, 3*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfig, 2*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsByFlagSet, 4*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsByFlagSet, 1*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsByFlagSets, 2*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfigByFlagSet, 5*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfigByFlagSets, 1*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfigByFlagSets, 2*time.Second)
	producer2.RecordLatency(telemetry.TreatmentsWithConfigByFlagSets, 3*time.Second)
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

	l1TreatmentsByFlagSet := int64(0)
	for _, count := range latsForM1.TreatmentsByFlagSet {
		l1TreatmentsByFlagSet += count
	}
	if l1TreatmentsByFlagSet != int64(1) {
		t.Error("latency count for .TreatmentsByFlagSet should be 1. Is: ", l1TreatmentsByFlagSet)
	}

	l1TreatmentsByFlagSets := int64(0)
	for _, count := range latsForM1.TreatmentsByFlagSets {
		l1TreatmentsByFlagSets += count
	}
	if l1TreatmentsByFlagSets != int64(1) {
		t.Error("latency count for .TreatmentsByFlagSet should be 1. Is: ", l1TreatmentsByFlagSets)
	}

	l1TreatmentsWithConfigByFlagSet := int64(0)
	for _, count := range latsForM1.TreatmentsWithConfigByFlagSet {
		l1TreatmentsWithConfigByFlagSet += count
	}
	if l1TreatmentsWithConfigByFlagSet != int64(2) {
		t.Error("latency count for .TreatmentsWithConfigByFlagSet should be 2. Is: ", l1TreatmentsWithConfigByFlagSet)
	}

	l1TreatmentsWithConfigByFlagSets := int64(0)
	for _, count := range latsForM1.TreatmentsWithConfigByFlagSets {
		l1TreatmentsWithConfigByFlagSets += count
	}
	if l1TreatmentsWithConfigByFlagSets != int64(1) {
		t.Error("latency count for .TreatmentsWithConfigByFlagSets should be 1. Is: ", l1TreatmentsWithConfigByFlagSets)
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

	l2TreatmentsByFlagSet := int64(0)
	for _, count := range latsForM2.TreatmentsByFlagSet {
		l2TreatmentsByFlagSet += count
	}
	if l2TreatmentsByFlagSet != int64(2) {
		t.Error("latency count for .TreatmentsByFlagSet should be 1. Is: ", l2TreatmentsByFlagSet)
	}

	l2TreatmentsByFlagSets := int64(0)
	for _, count := range latsForM2.TreatmentsByFlagSets {
		l2TreatmentsByFlagSets += count
	}
	if l2TreatmentsByFlagSets != int64(1) {
		t.Error("latency count for .TreatmentsByFlagSet should be 1. Is: ", l2TreatmentsByFlagSets)
	}

	l2TreatmentsWithConfigByFlagSet := int64(0)
	for _, count := range latsForM2.TreatmentsWithConfigByFlagSet {
		l2TreatmentsWithConfigByFlagSet += count
	}
	if l2TreatmentsWithConfigByFlagSet != int64(1) {
		t.Error("latency count for .TreatmentsWithConfigByFlagSet should be 1. Is: ", l2TreatmentsWithConfigByFlagSet)
	}

	l2TreatmentsWithConfigByFlagSets := int64(0)
	for _, count := range latsForM2.TreatmentsWithConfigByFlagSets {
		l2TreatmentsWithConfigByFlagSets += count
	}
	if l2TreatmentsWithConfigByFlagSets != int64(3) {
		t.Error("latency count for .TreatmentsWithConfigByFlagSets should be 3. Is: ", l2TreatmentsWithConfigByFlagSets)
	}

	l2Track := int64(0)
	for _, count := range latsForM2.Track {
		l2Track += count
	}
	if l2Track != 1 {
		t.Error("latency count for .Track should be 1. Is: ", l2Track)
	}
}

func TestPopConfigsPopulatedFromStorage(t *testing.T) {
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

func TestPopConfigsFromHash(t *testing.T) {
	redisPrefix, _ := getCurrentFuncName()
	innerClient, _ := redis.NewClient(&redis.UniversalOptions{})
	client, _ := redis.NewPrefixedRedisClient(innerClient, redisPrefix)
	defer func() {
		keys, _ := innerClient.Keys(redisPrefix + "*").Multi()
		innerClient.Del(keys...)
	}()

	err := client.HSet(redisSt.KeyInit, "go-3.4/myName/1.1.1.1", `{"om": 4}`)
	err = client.HSet(redisSt.KeyInit, "go-3.4/myName/1.1.1.1", `{"om": 4}`)
	err = client.HSet(redisSt.KeyInit, "go-3.4/myName/1.1.1.1", `{"om": 4}`)
	err = client.HSet(redisSt.KeyInit, "go-3.4/myName/1.1.1.2", `{"om": 7}`)
	if err != nil {
		t.Error(err)
	}

	logger := logging.NewLogger(nil)
	consumer := NewRedisTelemetryCosumerclient(client, logger)
	cfgs := consumer.PopConfigs()
	if len(cfgs) != 2 {
		t.Error("there should be 2 configs only (one per metadata")
	}

	if v, ok := cfgs[dtos.Metadata{SDKVersion: "go-3.4", MachineIP: "1.1.1.1", MachineName: "myName"}]; !ok || v.OperationMode != 4 {
		t.Error("wrong data")
	}

	if v, ok := cfgs[dtos.Metadata{SDKVersion: "go-3.4", MachineIP: "1.1.1.2", MachineName: "myName"}]; !ok || v.OperationMode != 7 {
		t.Error("wrong data")
	}
}

func TestPopConfigsFromList(t *testing.T) {
	redisPrefix, _ := getCurrentFuncName()
	innerClient, _ := redis.NewClient(&redis.UniversalOptions{})
	client, _ := redis.NewPrefixedRedisClient(innerClient, redisPrefix)
	defer func() {
		keys, _ := innerClient.Keys(redisPrefix + "*").Multi()
		innerClient.Del(keys...)
	}()

	toPush := []dtos.TelemetryQueueObject{
		{Metadata: dtos.Metadata{SDKVersion: "a", MachineName: "b", MachineIP: "c"}, Config: dtos.Config{OperationMode: 4}},
		{Metadata: dtos.Metadata{SDKVersion: "d", MachineName: "e", MachineIP: "f"}, Config: dtos.Config{OperationMode: 7}},
	}
	for _, cfg := range toPush {
		serialized, _ := json.Marshal(cfg)
		client.RPush(redisSt.KeyConfig, serialized)
	}

	logger := logging.NewLogger(nil)
	consumer := NewRedisTelemetryCosumerclient(client, logger)
	cfgs := consumer.PopConfigs()
	if len(cfgs) != 2 {
		t.Error("there should be 2 configs only (one per metadata)")
		t.Error(cfgs)
	}

	if v, ok := cfgs[dtos.Metadata{SDKVersion: "a", MachineIP: "c", MachineName: "b"}]; !ok || v.OperationMode != 4 {
		t.Error("wrong data")
	}

	if v, ok := cfgs[dtos.Metadata{SDKVersion: "d", MachineIP: "f", MachineName: "e"}]; !ok || v.OperationMode != 7 {
		t.Error("wrong data")
	}
}
