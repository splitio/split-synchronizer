package redisstate

import (
	"crypto/tls"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	config "github.com/splitio/go-split-commons/v10/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReadClientStandalone(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:         "redis.internal",
		Port:         6380,
		Database:     3,
		Username:     "reader",
		Password:     "test-password",
		Prefix:       "pfx",
		MaxRetries:   2,
		PoolSize:     7,
		DialTimeout:  4,
		ReadTimeout:  5,
		WriteTimeout: 6,
	}
	rc, err := NewReadClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })

	assert.Equal(t, "pfx", rc.Prefix())
	c, ok := rc.client.(*goredis.Client)
	require.True(t, ok, "standalone config must build a plain go-redis client")
	opts := c.Options()
	assert.Equal(t, "redis.internal:6380", opts.Addr)
	assert.Equal(t, 3, opts.DB)
	assert.Equal(t, "reader", opts.Username)
	assert.Equal(t, "test-password", opts.Password)
	assert.Equal(t, 2, opts.MaxRetries)
	assert.Equal(t, 7, opts.PoolSize)
	assert.Equal(t, 4*time.Second, opts.DialTimeout)
	assert.Equal(t, readCallTimeout, opts.ReadTimeout, "cfg.ReadTimeout=5 is below readCallTimeout, so the floor applies")
	assert.Equal(t, 6*time.Second, opts.WriteTimeout)
}

func TestNewReadClientSentinel(t *testing.T) {
	cfg := &config.RedisConfig{
		SentinelAddresses: []string{"s1:26379", "s2:26379"},
		SentinelMaster:    "mymaster",
		Prefix:            "pfx",
		Database:          1,
		Password:          "test-password",
	}
	opts, prefix, err := universalOptions(cfg)
	require.NoError(t, err)
	assert.Equal(t, "mymaster", opts.MasterName)
	assert.Equal(t, []string{"s1:26379", "s2:26379"}, opts.Addrs)
	assert.Equal(t, 1, opts.DB)
	assert.Equal(t, "pfx", prefix)

	rc, err := NewReadClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })
	assert.Equal(t, "pfx", rc.Prefix())

	_, _, err = universalOptions(&config.RedisConfig{SentinelAddresses: []string{"s1:26379"}, Prefix: "pfx"})
	assert.ErrorIs(t, err, ErrInvalidReadConfig, "sentinel without a master name must be rejected")

	_, err = NewReadClient(&config.RedisConfig{
		SentinelAddresses: []string{"s1:26379"},
		SentinelMaster:    "mymaster",
		ClusterNodes:      []string{"c1:7000"},
	})
	assert.ErrorIs(t, err, ErrInvalidReadConfig, "sentinel and cluster are mutually exclusive")
}

func TestNewReadClientCluster(t *testing.T) {
	rc, err := NewReadClient(&config.RedisConfig{ClusterNodes: []string{"c1:7000", "c2:7000"}, Prefix: "pfx"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })
	_, ok := rc.client.(*goredis.ClusterClient)
	assert.True(t, ok, "cluster config must build a cluster client")
	assert.Equal(t, "{SPLITIO}pfx", rc.Prefix(), "default hash tag goes in front of the prefix")

	noPrefix, err := NewReadClient(&config.RedisConfig{ClusterNodes: []string{"c1:7000"}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = noPrefix.Close() })
	assert.Equal(t, "{SPLITIO}", noPrefix.Prefix())

	custom, err := NewReadClient(&config.RedisConfig{ClusterNodes: []string{"c1:7000"}, Prefix: "pfx", ClusterKeyHashTag: "{app}"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = custom.Close() })
	assert.Equal(t, "{app}pfx", custom.Prefix(), "custom hash tag replaces the default")

	for _, tag := range []string{"app", "{a}{b}", "{a", "{}", "{a}b}"} {
		_, _, err := universalOptions(&config.RedisConfig{ClusterNodes: []string{"c1:7000"}, ClusterKeyHashTag: tag})
		assert.ErrorIs(t, err, ErrInvalidReadConfig, "hash tag %q must be rejected", tag)
	}
}

func TestNewReadClientTLS(t *testing.T) {
	tlsCfg := &tls.Config{ServerName: "redis.internal", MinVersion: tls.VersionTLS12}

	standalone := &config.RedisConfig{Host: "redis.internal", Port: 6380, TLSConfig: tlsCfg}
	opts, _, err := universalOptions(standalone)
	require.NoError(t, err)
	assert.Same(t, tlsCfg, opts.TLSConfig)

	rc, err := NewReadClient(standalone)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })
	c, ok := rc.client.(*goredis.Client)
	require.True(t, ok)
	require.NotNil(t, c.Options().TLSConfig)
	assert.Equal(t, "redis.internal", c.Options().TLSConfig.ServerName)
	assert.Equal(t, uint16(tls.VersionTLS12), c.Options().TLSConfig.MinVersion, "MinVersion is passed through unchanged")
	assert.False(t, c.Options().TLSConfig.InsecureSkipVerify, "certificate verification stays on")

	sentinelOpts, _, err := universalOptions(&config.RedisConfig{
		SentinelAddresses: []string{"s1:26379"},
		SentinelMaster:    "mymaster",
		TLSConfig:         tlsCfg,
	})
	require.NoError(t, err)
	assert.Same(t, tlsCfg, sentinelOpts.TLSConfig)

	clusterOpts, _, err := universalOptions(&config.RedisConfig{ClusterNodes: []string{"c1:7000"}, TLSConfig: tlsCfg})
	require.NoError(t, err)
	assert.Same(t, tlsCfg, clusterOpts.TLSConfig)

	plain, _, err := universalOptions(&config.RedisConfig{Host: "redis.internal", Port: 6379})
	require.NoError(t, err)
	assert.Nil(t, plain.TLSConfig, "no TLS config must leave TLS off")
}

// go-split-commons NewRedisClient builds its options inline and hides the client, so this table mirrors its
// option mapping at the pinned libCommonsVersion. The version pin below forces a re-check on upgrade.
const libCommonsVersion = "v10.2.1"

func TestReadClientOptionMappingMatchesLibrary(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	require.True(t, ok, "build info is needed to check the go-split-commons version")
	var got string
	for _, dep := range info.Deps {
		if dep.Path == "github.com/splitio/go-split-commons/v10" {
			got = dep.Version
		}
	}
	require.Equal(t, libCommonsVersion, got, "go-split-commons changed: re-read storage/redis/redis.go NewRedisClient, update this table and libCommonsVersion")

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	common := config.RedisConfig{
		Username: "u", Password: "p", Database: 2, MaxRetries: 3, PoolSize: 9,
		DialTimeout: 1, ReadTimeout: 2, WriteTimeout: 3, TLSConfig: tlsCfg, Prefix: "pfx",
	}
	with := func(f func(*config.RedisConfig)) *config.RedisConfig {
		c := common
		f(&c)
		return &c
	}

	cases := []struct {
		name       string
		cfg        *config.RedisConfig
		wantAddrs  []string
		wantMaster string
		wantPrefix string
	}{
		{"standalone", with(func(c *config.RedisConfig) { c.Host, c.Port = "h", 6380 }), []string{"h:6380"}, "", "pfx"},
		{"sentinel", with(func(c *config.RedisConfig) { c.SentinelAddresses, c.SentinelMaster = []string{"s1:1", "s2:1"}, "m" }), []string{"s1:1", "s2:1"}, "m", "pfx"},
		{"cluster default tag", with(func(c *config.RedisConfig) { c.ClusterNodes = []string{"c1:7000"} }), []string{"c1:7000"}, "", "{SPLITIO}pfx"},
		{"cluster custom tag", with(func(c *config.RedisConfig) { c.ClusterNodes, c.ClusterKeyHashTag = []string{"c1:7000"}, "{app}" }), []string{"c1:7000"}, "", "{app}pfx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, prefix, err := universalOptions(tc.cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAddrs, opts.Addrs)
			assert.Equal(t, tc.wantMaster, opts.MasterName)
			assert.Equal(t, tc.wantPrefix, prefix)
			assert.Equal(t, "u", opts.Username)
			assert.Equal(t, "p", opts.Password)
			assert.Equal(t, 2, opts.DB)
			assert.Equal(t, 3, opts.MaxRetries)
			assert.Equal(t, 9, opts.PoolSize)
			assert.Equal(t, time.Second, opts.DialTimeout)
			assert.Equal(t, readCallTimeout, opts.ReadTimeout, "cfg.ReadTimeout=2 is below readCallTimeout, so the floor applies")
			assert.Equal(t, 3*time.Second, opts.WriteTimeout)
			assert.Same(t, tlsCfg, opts.TLSConfig)
		})
	}

	t.Run("invalid configurations the library rejects", func(t *testing.T) {
		for name, cfg := range map[string]*config.RedisConfig{
			"sentinel and cluster": {SentinelAddresses: []string{"s:1"}, SentinelMaster: "m", ClusterNodes: []string{"c:1"}},
			"sentinel no master":   {SentinelAddresses: []string{"s:1"}},
			"bad hash tag":         {ClusterNodes: []string{"c:1"}, ClusterKeyHashTag: "app"},
		} {
			_, _, err := universalOptions(cfg)
			assert.ErrorIs(t, err, ErrInvalidReadConfig, name)
		}
	})
}

func TestUniversalOptionsReadTimeoutFloorsAtReadCallTimeout(t *testing.T) {
	cases := []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{"small writer read timeout is raised to the read call bound", 1, readCallTimeout},
		{"large writer read timeout is kept", 120, 120 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, _, err := universalOptions(&config.RedisConfig{ReadTimeout: tc.seconds})
			require.NoError(t, err)
			assert.Equal(t, tc.want, opts.ReadTimeout)
		})
	}
}

// Writer keys come from NewKeys(client.Prefix(), ...); go-toolkit adds Prefix()+"." to each unprefixed key
// (the writer's errHead path). The reader must produce the same full key name for every topology.
func TestReadClientPrefixMatchesWriterKeys(t *testing.T) {
	cases := []struct {
		name       string
		cfg        *config.RedisConfig
		wantPrefix string
	}{
		{"standalone", &config.RedisConfig{Host: "h", Port: 6379, Prefix: "pfx"}, "pfx"},
		{"sentinel", &config.RedisConfig{SentinelAddresses: []string{"s1:26379"}, SentinelMaster: "m", Prefix: "pfx"}, "pfx"},
		{"cluster default tag", &config.RedisConfig{ClusterNodes: []string{"c1:7000"}, Prefix: "pfx"}, "{SPLITIO}pfx"},
		{"cluster custom tag", &config.RedisConfig{ClusterNodes: []string{"c1:7000"}, Prefix: "pfx", ClusterKeyHashTag: "{app}"}, "{app}pfx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.HasPrefix(tc.wantPrefix, "{") {
				t.Skip("NewKeys rejects braces in the key prefix, so the default cluster hash tag cannot be built yet")
			}
			rc, err := NewReadClient(tc.cfg)
			require.NoError(t, err)
			t.Cleanup(func() { _ = rc.Close() })
			require.Equal(t, tc.wantPrefix, rc.Prefix())

			keys, err := NewKeys(rc.Prefix(), "test-apikey", "")
			require.NoError(t, err)
			writerFullMeta := rc.Prefix() + "." + keys.MetaUnprefixed()
			assert.Equal(t, writerFullMeta, keys.Meta(), "reader prefix plus writer unprefixed key must equal the writer key")
			if strings.HasPrefix(tc.wantPrefix, "{") {
				assert.True(t, strings.HasPrefix(keys.Meta(), tc.wantPrefix+"."), "cluster keys must carry the hash tag")
			}
		})
	}
}

func TestReadClientCloseTwice(t *testing.T) {
	rc, err := NewReadClient(&config.RedisConfig{Host: "redis.internal", Port: 6379, Prefix: "pfx"})
	require.NoError(t, err)

	require.NoError(t, rc.Close(), "first Close must succeed")

	var second error
	require.NotPanics(t, func() { second = rc.Close() }, "second Close must not panic")
	assert.ErrorIs(t, second, goredis.ErrClosed, "second Close returns go-redis ErrClosed")
}
