// Package redis implements different kind of storages for split information
package redis

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

func TestInitializeClient(t *testing.T) {

	config := conf.NewInitializedConfigData()
	Initialize(config.Redis)
	err := Client.Ping().Err()
	if err != nil {
		t.Error("Redis Client", err)
	}
}

func TestClusterAndSentinelEnabled(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.SentinelReplication = true
	config.Redis.ClusterMode = true
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "Incompatible configuration of redis, Sentinel and Cluster cannot be enabled at the same time" {
		t.Error("An error with message \"Missing redis sentinel master name\" should have been returned")
	}
}

func TestInitializeRedisSentinelWithoutMaster(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.SentinelReplication = true
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "Missing redis sentinel master name" {
		t.Error("An error with message \"Missing redis sentinel master name\" should have been returned")
	}
}

func TestInitializeRedisSentinelWithoutAddresses(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.SentinelReplication = true
	config.Redis.SentinelMaster = "someMaster"
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "Missing redis sentinels addresses" {
		t.Error("An error with message \"Missing redis sentinels urls\" should have been returned")
	}
}

func TestInitializeRedisSentinelProperly(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.SentinelReplication = true
	config.Redis.SentinelMaster = "someMaster"
	config.Redis.SentinelAddresses = "somehost:1234"
	err := Initialize(config.Redis)

	if err != nil {
		t.Error("No error should have been returned for valid sentinel parameters")
	}
}

func TestInitializeRedisClusterWithoutAddresses(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "Missing redis cluster addresses" {
		t.Error("An error with message \"Missing redis cluster urls\" should have been returned")
	}
}

func TestInitializeRedisClusterWithoutKeyHashTag(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	err := Initialize(config.Redis)

	if err != nil {
		t.Error("No error should have been returned for valid cluster parameters")
	}
}

func TestInitializeRedisClusterWithInvalidBeginingKeyHashTag(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	config.Redis.ClusterKeyHashTag = "{TEST"
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "keyHashTag is not valid" {
		t.Error("An error with message \"keyHashTag is not valid\" should have been returned")
	}
}

func TestInitializeRedisClusterWithInvalidEndingKeyHashTag(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	config.Redis.ClusterKeyHashTag = "TEST}"
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "keyHashTag is not valid" {
		t.Error("An error with message \"keyHashTag is not valid\" should have been returned")
	}
}

func TestInitializeRedisClusterWithInvalidLengthKeyHashTag(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	config.Redis.ClusterKeyHashTag = "{}"
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "keyHashTag is not valid" {
		t.Error("An error with message \"keyHashTag is not valid\" should have been returned")
	}
}

func TestInitializeRedisClusterWithInvalidKeyHashTag(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	config.Redis.ClusterKeyHashTag = "{TEST}}"
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "keyHashTag is not valid" {
		t.Error("An error with message \"keyHashTag is not valid\" should have been returned")
	}
}

func TestInitializeRedisClusterProperly(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterMode = true
	config.Redis.ClusterNodes = "somehost:1234"
	config.Redis.ClusterKeyHashTag = "{TEST}"
	err := Initialize(config.Redis)

	if err != nil {
		t.Error("No error should have been returned for valid cluster parameters")
	}
}

func TestImpressionsDrop(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.impressionsQueueNamespace())

	metadata := api.SdkMetadata{
		SdkVersion:  "test-2.0",
		MachineIP:   "127.0.0.1",
		MachineName: "ip-127-0-0-1",
	}

	impressionsRaw := map[string][]api.ImpressionDTO{
		"feature1": makeImpressions("key", "on", 123456, "some_label", "key", 30),
		"feature2": makeImpressions("key", "on", 123456, "some_label", "key", 70),
		"feature3": makeImpressions("key", "on", 123456, "some_label", "key", 100),
	}

	// Adding impressions to drop.
	for feature, impressions := range impressionsRaw {
		for _, impression := range impressions {
			toStore, err := json.Marshal(ImpressionDTO{
				Data: ImpressionObject{
					BucketingKey:      impression.BucketingKey,
					FeatureName:       feature,
					KeyName:           impression.KeyName,
					Rule:              impression.Label,
					SplitChangeNumber: impression.ChangeNumber,
					Timestamp:         impression.Time,
					Treatment:         impression.Treatment,
				},
				Metadata: ImpressionMetadata{
					InstanceIP:   metadata.MachineIP,
					InstanceName: metadata.MachineName,
					SdkVersion:   metadata.SdkVersion,
				},
			})
			if err != nil {
				t.Error(err.Error())
				return
			}

			Client.LPush(
				prefixAdapter.impressionsQueueNamespace(),
				toStore,
			)
		}
	}
	impressionsStorageAdapter := NewImpressionStorageAdapter(Client, "")
	var size int64 = 100
	err := impressionsStorageAdapter.Drop(&size)
	if err != nil {
		t.Error("It should not return error")
	}

	count := impressionsStorageAdapter.Size()
	if count != 100 {
		t.Error("It should kept 100 elements, not", count)
	}

	err = impressionsStorageAdapter.Drop(nil)
	if err != nil {
		t.Error("It should not return error")
	}
	count = impressionsStorageAdapter.Size()
	if count != 0 {
		t.Error("It should not be elements left")
	}

	Client.Del(prefixAdapter.impressionsQueueNamespace())
}

func TestEventsDrop(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	conf.Initialize()
	Initialize(conf.Data.Redis)
	prefixAdapter := &prefixAdapter{prefix: ""}
	Client.Del(prefixAdapter.eventsListNamespace())

	metadata := api.SdkMetadata{
		SdkVersion:  "test-2.0",
		MachineIP:   "127.0.0.1",
		MachineName: "ip-127-0-0-1",
	}

	eventsRaw := makeEvents("key", "test", 123456, "user", nil, 30)

	// Adding events to drop.
	for _, event := range eventsRaw {
		toStore, err := json.Marshal(api.RedisStoredEventDTO{
			Event: api.EventDTO{
				Key:             event.Key,
				EventTypeID:     event.EventTypeID,
				Timestamp:       event.Timestamp,
				TrafficTypeName: event.TrafficTypeName,
				Value:           event.Value,
			},
			Metadata: api.RedisStoredMachineMetadataDTO{
				MachineIP:   metadata.MachineIP,
				MachineName: metadata.MachineName,
				SDKVersion:  metadata.SdkVersion,
			},
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		Client.LPush(
			prefixAdapter.eventsListNamespace(),
			toStore,
		)
	}
	eventsStorageAdapter := NewEventStorageAdapter(Client, "")

	var size int64 = 9

	err := eventsStorageAdapter.Drop(&size)
	if err != nil {
		t.Error("It should not return error")
	}

	count := eventsStorageAdapter.Size()
	if count != 21 {
		t.Error("It should kept 19 elements, not", count)
	}

	err = eventsStorageAdapter.Drop(nil)
	if err != nil {
		t.Error("It should not return error")
	}
	count = eventsStorageAdapter.Size()
	if count != 0 {
		t.Error("It should not be elements left")
	}

	Client.Del(prefixAdapter.eventsListNamespace())
}
