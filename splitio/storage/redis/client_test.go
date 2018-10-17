// Package redis implements different kind of storages for split information
package redis

import (
	"testing"

	"github.com/splitio/split-synchronizer/conf"
)

func TestInitializeClient(t *testing.T) {

	config := conf.NewInitializedConfigData()
	Initialize(config.Redis)
	err := Client.Ping().Err()
	if err != nil {
		t.Error("Redis Client", err)
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
	config.Redis.ClusterReplication = true
	err := Initialize(config.Redis)

	if Client != nil {
		t.Error("Client should have been nil")
	}

	if err == nil || err.Error() != "Missing redis cluster addresses" {
		t.Error("An error with message \"Missing redis cluster urls\" should have been returned")
	}
}

func TestInitializeRedisClusterProperly(t *testing.T) {
	config := conf.NewInitializedConfigData()
	config.Redis.ClusterReplication = true
	config.Redis.ClusterAddresses = "somehost:1234"
	err := Initialize(config.Redis)

	if err != nil {
		t.Error("No error should have been returned for valid cluster parameters")
	}
}
