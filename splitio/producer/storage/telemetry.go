package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/redis"

	"github.com/splitio/go-split-commons/v4/dtos"
	redisSt "github.com/splitio/go-split-commons/v4/storage/redis"
	"github.com/splitio/go-split-commons/v4/telemetry"
)

const (
	configFetchBulkSize = 1000
)

// MultiMethodLatencies is a type alias mapping method latencies for multiple sdk instances
type MultiMethodLatencies = map[dtos.Metadata]dtos.MethodLatencies

// MultiMethodExceptions is a type alias mapping method exceptions for multiple sdk instances
type MultiMethodExceptions = map[dtos.Metadata]dtos.MethodExceptions

// MultiConfigs is a type alias mapping configs for multiple sdk instances
type MultiConfigs = map[dtos.Metadata]dtos.Config

// RedisTelemetryConsumerMulti interface
type RedisTelemetryConsumerMulti interface {
	PopLatencies() MultiMethodLatencies
	PopExceptions() MultiMethodExceptions
	PopConfigs() MultiConfigs
}

// RedisTelemetryConsumerMultiImpl implementation
type RedisTelemetryConsumerMultiImpl struct {
	client *redis.PrefixedRedisClient
	logger logging.LoggerInterface
}

// NewRedisTelemetryCosumerclient instantiates a redis sdk telemetry consumer
func NewRedisTelemetryCosumerclient(client *redis.PrefixedRedisClient, logger logging.LoggerInterface) *RedisTelemetryConsumerMultiImpl {
	return &RedisTelemetryConsumerMultiImpl{
		client: client,
		logger: logger,
	}
}

// PopLatencies extracts the latencies mapped by sdk instance metadata
func (r *RedisTelemetryConsumerMultiImpl) PopLatencies() MultiMethodLatencies {
	kv, err := r.client.HGetAll(redisSt.KeyLatency)
	if err != nil {
		r.logger.Error("Error fetching latencies for SDK Methods: ", err)
		return nil
	}

	r.client.Del(redisSt.KeyLatency)

	toRet := make(MultiMethodLatencies)
	for field, count := range kv {
		metadata, method, bucket, err := parseLatencyField(field)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Ignoring invalid latency field: '%s': %s", field, err.Error()))
			continue
		}

		intCount, err := strconv.ParseInt(count, 10, 64)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Ignoring latency with invalid count '%s'. Error: %s", count, err.Error()))
			continue
		}

		err = setLatency(toRet, metadata, method, bucket, intCount)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Could not register latency for field: '%s', count: %d. Error: %s", field, intCount, err.Error()))
		}
	}

	return toRet
}

// PopExceptions extracts the exception mapped by sdk instance metadata
func (r *RedisTelemetryConsumerMultiImpl) PopExceptions() MultiMethodExceptions {
	kv, err := r.client.HGetAll(redisSt.KeyException)
	if err != nil {
		r.logger.Error("Error fetching exceptions for SDK Methods: ", err)
		return nil
	}

	r.client.Del(redisSt.KeyException)

	toRet := make(MultiMethodExceptions)
	for field, count := range kv {
		metadata, method, err := parseExceptionField(field)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Ignoring invalid exception field: '%s': %s", field, err.Error()))
			continue
		}

		intCount, err := strconv.ParseInt(count, 10, 64)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Ignoring exception with invalid count '%s'. Error: %s", count, err.Error()))
			continue
		}

		err = setException(toRet, metadata, method, intCount)
		if err != nil {
			r.logger.Error(fmt.Sprintf("Could not register exception for field: '%s', count: %d. Error: %s", field, intCount, err.Error()))
		}
	}

	return toRet
}

// PopConfigs fetches and deletes accumulated configs from redis
func (r *RedisTelemetryConsumerMultiImpl) PopConfigs() MultiConfigs {
	toRet := make(MultiConfigs)

	for {
		data, done, err := fetchConfigsGreedy(r.client, configFetchBulkSize)
		if err != nil {
			r.logger.Error("Error fetching SDK configs from ready: ", err.Error())
			return toRet // return what's been fetched so far
		}

		dedupeAndAdd(toRet, data)

		if done {
			return toRet
		}
	}
}

func parseMetadata(field string) (*dtos.Metadata, error) {
	parts := strings.Split(field, redisSt.FieldSeparator)
	if l := len(parts); l != 3 {
		return nil, fmt.Errorf("invalid subsection count. Expected 3, got: %d", l)
	}

	return &dtos.Metadata{
		SDKVersion:  parts[redisSt.FieldLatencyIndexSdkVersion],
		MachineName: parts[redisSt.FieldLatencyIndexMachineName],
		MachineIP:   parts[redisSt.FieldLatencyIndexMachineIP],
	}, nil
}

func parseLatencyField(field string) (metadata *dtos.Metadata, method string, bucket int, err error) {
	parts := strings.Split(field, redisSt.FieldSeparator)
	if l := len(parts); l != 5 {
		return nil, "", 0, fmt.Errorf("invalid subsection count. Expected 5, got: %d", l)
	}

	if !telemetry.IsMethodValid(&parts[redisSt.FieldLatencyIndexMethod]) {
		return nil, "", 0, fmt.Errorf("unknown method '%s'", parts[redisSt.FieldLatencyIndexMethod])
	}

	intBucket, err := strconv.ParseInt(parts[redisSt.FieldLatencyIndexBucket], 10, 64)
	if err != nil {
		return nil, "", 0, fmt.Errorf("error parsing count: %w", err)
	}

	return &dtos.Metadata{
		SDKVersion:  parts[redisSt.FieldLatencyIndexSdkVersion],
		MachineName: parts[redisSt.FieldLatencyIndexMachineName],
		MachineIP:   parts[redisSt.FieldLatencyIndexMachineIP],
	}, parts[redisSt.FieldLatencyIndexMethod], int(intBucket), nil
}

func setLatency(result MultiMethodLatencies, metadata *dtos.Metadata, method string, bucket int, count int64) error {
	if bucket >= telemetry.LatencyBucketCount {
		return fmt.Errorf("'%d' exceeds max latency buckets '%d'", bucket, telemetry.LatencyBucketCount)
	}

	if _, ok := result[*metadata]; !ok {
		result[*metadata] = dtos.MethodLatencies{
			Treatment:            make([]int64, telemetry.LatencyBucketCount),
			Treatments:           make([]int64, telemetry.LatencyBucketCount),
			TreatmentWithConfig:  make([]int64, telemetry.LatencyBucketCount),
			TreatmentsWithConfig: make([]int64, telemetry.LatencyBucketCount),
			Track:                make([]int64, telemetry.LatencyBucketCount),
		}
	}

	switch method {
	case telemetry.Treatment:
		result[*metadata].Treatment[bucket] = count
	case telemetry.Treatments:
		result[*metadata].Treatments[bucket] = count
	case telemetry.TreatmentWithConfig:
		result[*metadata].TreatmentWithConfig[bucket] = count
	case telemetry.TreatmentsWithConfig:
		result[*metadata].TreatmentsWithConfig[bucket] = count
	case telemetry.Track:
		result[*metadata].Track[bucket] = count
	default:
		return fmt.Errorf("unknown method '%s'", method)
	}

	return nil
}

func parseExceptionField(field string) (metadata *dtos.Metadata, method string, err error) {
	parts := strings.Split(field, redisSt.FieldSeparator)
	if l := len(parts); l != 4 {
		return nil, "", fmt.Errorf("invalid subsection count. Expected 5, got: %d", l)
	}

	if !telemetry.IsMethodValid(&parts[redisSt.FieldExceptionIndexMethod]) {
		return nil, "", fmt.Errorf("unknown method '%s'", parts[redisSt.FieldExceptionIndexMethod])
	}

	return &dtos.Metadata{
		SDKVersion:  parts[redisSt.FieldExceptionIndexSdkVersion],
		MachineName: parts[redisSt.FieldExceptionIndexMachineName],
		MachineIP:   parts[redisSt.FieldExceptionIndexMachineIP],
	}, parts[redisSt.FieldExceptionIndexMethod], nil
}

func setException(result MultiMethodExceptions, metadata *dtos.Metadata, method string, count int64) error {
	curr := result[*metadata]
	switch method {
	case telemetry.Treatment:
		curr.Treatment = count
	case telemetry.Treatments:
		curr.Treatments = count
	case telemetry.TreatmentWithConfig:
		curr.TreatmentWithConfig = count
	case telemetry.TreatmentsWithConfig:
		curr.TreatmentsWithConfig = count
	case telemetry.Track:
		curr.Track = count
	default:
		return fmt.Errorf("unknown method '%s'", method)
	}

	result[*metadata] = curr
	return nil
}

func fetchConfigsGreedy(rclient *redis.PrefixedRedisClient, limit int64) (data []dtos.TelemetryQueueObject, done bool, err error) {

	kt, err := rclient.Type(redisSt.KeyConfig)
	if err != nil {
		return nil, false, fmt.Errorf("error determining redis configs key type: %w", err)
	}

	switch kt {
	case "list":
		raws, err := rclient.LRange(redisSt.KeyConfig, 0, limit)
		if err != nil {
			return nil, false, fmt.Errorf("error fetching configs from redis: %w", err)
		}

		if len(raws) == 0 {
			return nil, true, nil
		}

		err = rclient.LTrim(redisSt.KeyConfig, int64(len(raws)), -1)
		if err != nil {
			// Since we failed to delete the entries from redis, we fail early, so that we don't post them multiple times.
			return nil, false, fmt.Errorf("error deleting fetched configs from redis: %w", err)
		}

		toRet := make([]dtos.TelemetryQueueObject, 0, len(raws))
		for _, raw := range raws {
			var parsed dtos.TelemetryQueueObject
			err = json.Unmarshal([]byte(raw), &parsed)
			if err != nil {
				// TODO(mredolatti): Log?
				continue
			}
			toRet = append(toRet, parsed)
		}
		return toRet, (len(raws) < int(limit)), nil
	case "hash":
		raws, err := rclient.HGetAll(redisSt.KeyConfig)
		if err != nil {
			return nil, false, fmt.Errorf("error fetching configs from redis: %w", err)
		}

		if len(raws) == 0 {
			return nil, true, nil
		}

		toRet := make([]dtos.TelemetryQueueObject, 0, len(raws))
		for rawMeta, raw := range raws {
			meta, err := parseMetadata(rawMeta)
			if err != nil {
				// TODO(mredolatti): Log?
				continue
			}

			var parsed dtos.Config
			err = json.Unmarshal([]byte(raw), &parsed)
			if err != nil {
				// TODO(mredolatti): Log?
				continue
			}
			toRet = append(toRet, dtos.TelemetryQueueObject{Metadata: *meta, Config: parsed})
		}

		_, err = rclient.Del(redisSt.KeyConfig)
		if err != nil {
			// Since we failed to delete the entries from redis, we fail early, so that we don't post them multiple times.
			return nil, false, fmt.Errorf("error deleting fetched configs from redis: %w", err)
		}
		return toRet, true, nil
	case "none":
		// No metrics found
		return nil, true, nil
	}
	return nil, false, fmt.Errorf("invalid config key type: '%s'", kt)

}

func dedupeAndAdd(toRet MultiConfigs, data []dtos.TelemetryQueueObject) {
	for _, dto := range data { // The map will only keep the last version of a config object for a particular metadata
		toRet[dto.Metadata] = dto.Config
	}
}
