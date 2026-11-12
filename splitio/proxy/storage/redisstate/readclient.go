package redisstate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	config "github.com/splitio/go-split-commons/v10/conf"
)

// ErrInvalidReadConfig is returned when the Redis configuration cannot be turned into a read client.
var ErrInvalidReadConfig = errors.New("invalid redis configuration for reading the redis state")

// defaultClusterHashTag matches the go-split-commons cluster default. The writer and reader must derive the same
// prefix.
const defaultClusterHashTag = "{SPLITIO}"

// RedisReadClient reads redis state with go-redis directly; go-toolkit's prefixed client lacks ZSCAN and HGETALL for
// this layout. Keys passed in already carry the prefix from Prefix(). Close releases the pool.
type RedisReadClient struct {
	client goredis.UniversalClient
	prefix string
}

// NewReadClient builds a read client from the Redis config (standalone, sentinel, cluster, TLS). It opens no
// connection until the first command.
func NewReadClient(cfg *config.RedisConfig) (*RedisReadClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: nil config", ErrInvalidReadConfig)
	}
	opts, prefix, err := universalOptions(cfg)
	if err != nil {
		return nil, err
	}
	if len(cfg.ClusterNodes) > 0 && len(cfg.SentinelAddresses) == 0 {
		return &RedisReadClient{client: goredis.NewClusterClient(opts.Cluster()), prefix: prefix}, nil
	}
	return &RedisReadClient{client: goredis.NewUniversalClient(opts), prefix: prefix}, nil
}

// Prefix returns the writer's key prefix, with the cluster hash tag in front in cluster mode. Build Keys with it.
func (c *RedisReadClient) Prefix() string {
	return c.prefix
}

// HGetAll returns every field and value of the hash at key.
func (c *RedisReadClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// ZScan returns one page of the sorted set at key as flattened member, score pairs, and the next cursor (0 when
// complete).
func (c *RedisReadClient) ZScan(ctx context.Context, key string, cursor uint64, count int64) ([]string, uint64, error) {
	return c.client.ZScan(ctx, key, cursor, "", count).Result()
}

// HGet returns one hash field. found is false when the key or the field does not exist.
func (c *RedisReadClient) HGet(ctx context.Context, key, field string) (string, bool, error) {
	v, err := c.client.HGet(ctx, key, field).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Close releases the connection pool. Close the client once; the caller owns it. A second call does not panic and
// returns goredis.ErrClosed ("redis: client is closed").
func (c *RedisReadClient) Close() error {
	return c.client.Close()
}

// universalOptions maps the config to go-redis options and returns the writer's prefix. It mirrors go-split-commons
// NewRedisClient, which does not export this mapping.
func universalOptions(cfg *config.RedisConfig) (*goredis.UniversalOptions, string, error) {
	if len(cfg.SentinelAddresses) > 0 && len(cfg.ClusterNodes) > 0 {
		return nil, "", fmt.Errorf("%w: sentinel and cluster are mutually exclusive", ErrInvalidReadConfig)
	}

	// The socket read timeout must not be shorter than readCallTimeout: a single HGETALL of a very large flag hash
	// can take longer than the writer's ReadTimeout (default 10s) and would fail with i/o timeout at boot.
	readTimeout := time.Duration(cfg.ReadTimeout) * time.Second
	if readTimeout < readCallTimeout {
		readTimeout = readCallTimeout
	}

	opts := &goredis.UniversalOptions{
		Password:     cfg.Password,
		Username:     cfg.Username,
		DB:           cfg.Database,
		TLSConfig:    cfg.TLSConfig,
		MaxRetries:   cfg.MaxRetries,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:  readTimeout,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
	}
	prefix := cfg.Prefix

	switch {
	case len(cfg.SentinelAddresses) > 0:
		if cfg.SentinelMaster == "" {
			return nil, "", fmt.Errorf("%w: sentinel master name is required", ErrInvalidReadConfig)
		}
		opts.MasterName = cfg.SentinelMaster
		opts.Addrs = cfg.SentinelAddresses
	case len(cfg.ClusterNodes) > 0:
		tag, err := clusterHashTag(cfg.ClusterKeyHashTag)
		if err != nil {
			return nil, "", err
		}
		prefix = tag + prefix
		opts.Addrs = cfg.ClusterNodes
	default:
		opts.Addrs = []string{net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))}
	}
	return opts, prefix, nil
}

// clusterHashTag returns the configured hash tag or the default. A custom tag must be exactly one {...} pair.
func clusterHashTag(tag string) (string, error) {
	if tag == "" {
		return defaultClusterHashTag, nil
	}
	if len(tag) < 3 || tag[0] != '{' || tag[len(tag)-1] != '}' ||
		strings.Count(tag, "{") != 1 || strings.Count(tag, "}") != 1 {
		return "", fmt.Errorf("%w: cluster key hash tag must be one {...} pair", ErrInvalidReadConfig)
	}
	return tag, nil
}
