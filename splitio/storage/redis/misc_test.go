package redis

import (
	"github.com/splitio/split-synchronizer/conf"
	"testing"
)

func TestGetApikeyHash(t *testing.T) {
	conf.Initialize()
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	Initialize(conf.Data.Redis)
	Client.Set("some_prefix.SPLITIO.hash", "3376912823", 0)
	miscStorarage := NewMiscStorageAdapter(Client, "some_prefix")
	if str, _ := miscStorarage.GetApikeyHash(); str != "3376912823" {
		t.Error("Invalid hash fetched!")
	}

	Client.Del("some_prefix.SPLITIO.hash")
}

func TestSetApikeyHash(t *testing.T) {
	conf.Initialize()
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	Initialize(conf.Data.Redis)
	miscStorarage := NewMiscStorageAdapter(Client, "some_prefix")
	miscStorarage.SetApikeyHash("12345678")
	if apikeyHash := Client.Get("some_prefix.SPLITIO.hash").Val(); apikeyHash != "12345678" {
		t.Error("Invalid hash fetched!")
	}

	Client.Del("some_prefix.SPLITIO.hash")
}

func TestClearAll(t *testing.T) {
	conf.Initialize()
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	Initialize(conf.Data.Redis)

	Client.Set("some_prefix.SPLITIO.split.feature1", "asd", 0)
	Client.Set("some_prefix.SPLITIO.hash", "3376912823", 0)
	Client.Set("some_prefix.SPLITIO.impressions", "abc", 0)
	Client.Set("some_prefix.SPLITIO.events", "der", 0)
	Client.Set("some_prefix.SPLITIO.splits.till", "-1", 0)

	miscStorarage := NewMiscStorageAdapter(Client, "some_prefix")
	miscStorarage.ClearAll()

	if keys := Client.Keys("some_prefix.SPLITIO*").Val(); len(keys) != 0 {
		t.Error("Everything should have been wiped")
	}
}
