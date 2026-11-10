package common

import (
	"crypto/tls"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"

	"github.com/stretchr/testify/assert"
)

func TestParseRedisOptions(t *testing.T) {
	t.Run("standalone", func(t *testing.T) {
		cfg := &conf.Redis{Host: "redis.local", Port: 1234, Db: 2, Network: "tcp", PoolSize: 5}
		opts, err := ParseRedisOptions(cfg)
		assert.NoError(t, err)
		assert.Equal(t, "redis.local", opts.Host)
		assert.Equal(t, 1234, opts.Port)
		assert.Equal(t, 2, opts.Database)
		assert.Empty(t, opts.SentinelAddresses)
		assert.Empty(t, opts.ClusterNodes)
		assert.Nil(t, opts.TLSConfig)
	})

	t.Run("sentinel wins over cluster and standalone", func(t *testing.T) {
		cfg := &conf.Redis{
			Host:                "redis.local",
			SentinelReplication: true,
			SentinelAddresses:   "s1:26379,s2:26379",
			SentinelMaster:      "mymaster",
			ClusterMode:         true,
			ClusterNodes:        "c1:6379",
		}
		opts, err := ParseRedisOptions(cfg)
		assert.NoError(t, err)
		assert.Equal(t, []string{"s1:26379", "s2:26379"}, opts.SentinelAddresses)
		assert.Equal(t, "mymaster", opts.SentinelMaster)
		assert.Empty(t, opts.ClusterNodes)
		assert.Empty(t, opts.Host)
	})

	t.Run("cluster wins over standalone", func(t *testing.T) {
		cfg := &conf.Redis{
			Host:              "redis.local",
			ClusterMode:       true,
			ClusterNodes:      "c1:6379,c2:6379",
			ClusterKeyHashTag: "{SPLITIO}",
		}
		opts, err := ParseRedisOptions(cfg)
		assert.NoError(t, err)
		assert.Equal(t, []string{"c1:6379", "c2:6379"}, opts.ClusterNodes)
		assert.Equal(t, "{SPLITIO}", opts.ClusterKeyHashTag)
		assert.Empty(t, opts.Host)
	})

	t.Run("tls disabled yields no tls config", func(t *testing.T) {
		opts, err := ParseRedisOptions(&conf.Redis{Host: "h", Port: 6379})
		assert.NoError(t, err)
		assert.Nil(t, opts.TLSConfig)
	})

	t.Run("tls with missing CA file fails", func(t *testing.T) {
		cfg := &conf.Redis{Host: "h", TLS: true, TLSCACertificates: []string{"/does/not/exist/ca.pem"}}
		_, err := ParseRedisOptions(cfg)
		assert.Error(t, err)
	})

	t.Run("tls client key without certificate fails", func(t *testing.T) {
		cfg := &conf.Redis{Host: "h", TLS: true, TLSClientKey: "key.pem"}
		_, err := ParseRedisOptions(cfg)
		assert.Error(t, err)
	})
}

func TestParseTLSConfigMinVersion12(t *testing.T) {
	cfg, err := parseTLSConfig(&conf.Redis{TLS: true, Host: "redis.local"})
	assert.NoError(t, err)
	if assert.NotNil(t, cfg) {
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	}
}
