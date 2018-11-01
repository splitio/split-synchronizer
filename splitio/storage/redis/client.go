package redis

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/splitio/split-synchronizer/conf"
)

// Client is a redis client with a connection pool
var Client redis.UniversalClient

// BaseStorageAdapter basic redis storage adapter
type BaseStorageAdapter struct {
	*prefixAdapter
	client redis.UniversalClient
}

// Initialize Redis module with a pool connection
func Initialize(redisOptions conf.RedisSection) error {
	var err error
	Client, err = NewInstance(redisOptions)
	return err
}

// NewInstance returns an instance of Redis Client
func NewInstance(opt conf.RedisSection) (redis.UniversalClient, error) {
	if opt.SentinelReplication && opt.ClusterMode {
		return nil, errors.New("Incompatible configuration of redis, Sentinel and Cluster cannot be enabled at the same time")
	}

	if opt.SentinelReplication {
		if opt.SentinelMaster == "" {
			return nil, errors.New("Missing redis sentinel master name")
		}

		if opt.SentinelAddresses == "" {
			return nil, errors.New("Missing redis sentinels addresses")
		}

		addresses := strings.Split(opt.SentinelAddresses, ",")

		return redis.NewUniversalClient(
			&redis.UniversalOptions{
				MasterName:   opt.SentinelMaster,
				Addrs:        addresses,
				Password:     opt.Pass,
				DB:           opt.Db,
				MaxRetries:   opt.MaxRetries,
				PoolSize:     opt.PoolSize,
				DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
				ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
				WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
			}), nil
	}

	if opt.ClusterMode {
		if opt.ClusterNodes == "" {
			return nil, errors.New("Missing redis cluster addresses")
		}

		addresses := strings.Split(opt.ClusterNodes, ",")

		return redis.NewUniversalClient(
			&redis.UniversalOptions{
				Addrs:        addresses,
				Password:     opt.Pass,
				PoolSize:     opt.PoolSize,
				DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
				ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
				WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
			}), nil
	}

	return redis.NewUniversalClient(
		&redis.UniversalOptions{
			// Network:      opt.Network,
			Addrs:        []string{fmt.Sprintf("%s:%d", opt.Host, opt.Port)},
			Password:     opt.Pass,
			DB:           opt.Db,
			MaxRetries:   opt.MaxRetries,
			PoolSize:     opt.PoolSize,
			DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
			ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
		}), nil
}
