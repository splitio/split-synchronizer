// Package redis implements different kind of storages for split information
package redis

import (
	"testing"

	"github.com/splitio/go-agent/conf"
)

func TestInitializeClient(t *testing.T) {

	config := conf.NewInitializedConfigData()
	Initialize(config.Redis)
	err := Client.Ping().Err()
	if err != nil {
		t.Error("Redis Client", err)
	}
}
