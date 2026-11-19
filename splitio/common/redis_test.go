package common

import (
	"crypto/tls"
	"fmt"
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

func TestParseRedisOptionsTrimsAddressLists(t *testing.T) {
	sentinel, err := ParseRedisOptions(&conf.Redis{SentinelReplication: true, SentinelAddresses: "a:1, b:2", SentinelMaster: "m"})
	assert.NoError(t, err)
	assert.Equal(t, []string{"a:1", "b:2"}, sentinel.SentinelAddresses)

	cluster, err := ParseRedisOptions(&conf.Redis{ClusterMode: true, ClusterNodes: " a:1 ,,b:2, "})
	assert.NoError(t, err)
	assert.Equal(t, []string{"a:1", "b:2"}, cluster.ClusterNodes)
}

func TestParseRedisOptionsEmptyAddressListsAreErrors(t *testing.T) {
	_, err := ParseRedisOptions(&conf.Redis{SentinelReplication: true, SentinelAddresses: " , ", SentinelMaster: "m"})
	assert.EqualError(t, err, "sentinel addresses must not be empty")

	_, err = ParseRedisOptions(&conf.Redis{ClusterMode: true, ClusterNodes: ""})
	assert.EqualError(t, err, "cluster nodes must not be empty")
}

func TestParseRedisOptionsSentinelMasterValidation(t *testing.T) {
	t.Run("blank master is rejected", func(t *testing.T) {
		_, err := ParseRedisOptions(&conf.Redis{SentinelReplication: true, SentinelAddresses: "s1:26379", SentinelMaster: "  "})
		assert.EqualError(t, err, "sentinel master must not be empty")
	})

	t.Run("master is stored trimmed", func(t *testing.T) {
		opts, err := ParseRedisOptions(&conf.Redis{SentinelReplication: true, SentinelAddresses: "s1:26379", SentinelMaster: " mymaster "})
		assert.NoError(t, err)
		assert.Equal(t, "mymaster", opts.SentinelMaster)
	})
}

func TestParseRedisOptionsPortValidation(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		t.Run(fmt.Sprintf("rejects port %d", port), func(t *testing.T) {
			_, err := ParseRedisOptions(&conf.Redis{Host: "h", Port: port})
			assert.EqualError(t, err, fmt.Sprintf("redis port must be 1..65535, got %d", port))
		})
	}

	for _, port := range []int{1, 6379, 65535} {
		t.Run(fmt.Sprintf("accepts port %d", port), func(t *testing.T) {
			opts, err := ParseRedisOptions(&conf.Redis{Host: "h", Port: port})
			assert.NoError(t, err)
			assert.Equal(t, port, opts.Port)
		})
	}
}

func TestParseRedisOptionsTLSServerNameDefault(t *testing.T) {
	cases := []struct {
		name     string
		cfg      conf.Redis
		expected string
	}{
		{
			name:     "sentinel first address with port",
			cfg:      conf.Redis{TLS: true, SentinelReplication: true, SentinelAddresses: "s1.example:26379, s2.example:26379", SentinelMaster: "m"},
			expected: "s1.example",
		},
		{
			name:     "sentinel first address without port",
			cfg:      conf.Redis{TLS: true, SentinelReplication: true, SentinelAddresses: "s1.example", SentinelMaster: "m"},
			expected: "s1.example",
		},
		{
			name:     "cluster first node with port",
			cfg:      conf.Redis{TLS: true, ClusterMode: true, ClusterNodes: "c1.example:6379,c2.example:6379"},
			expected: "c1.example",
		},
		{
			name:     "explicit server name wins",
			cfg:      conf.Redis{TLS: true, TLSServerName: "custom.example", ClusterMode: true, ClusterNodes: "c1.example:6379"},
			expected: "custom.example",
		},
		{
			name:     "host wins over addresses",
			cfg:      conf.Redis{TLS: true, Host: "h.example", ClusterMode: true, ClusterNodes: "c1.example:6379"},
			expected: "h.example",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			tlsCfg, err := parseTLSConfig(&cfg)
			assert.NoError(t, err)
			if assert.NotNil(t, tlsCfg) {
				assert.Equal(t, tc.expected, tlsCfg.ServerName)
			}
		})
	}
}
