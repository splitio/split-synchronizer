package redis

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/splitio/split-synchronizer/conf"
	redis "gopkg.in/redis.v5"
)

// Client is a redis client with a connection pool
var Client *redis.Client

// BaseStorageAdapter basic redis storage adapter
type BaseStorageAdapter struct {
	*prefixAdapter
	client *redis.Client
}

// Initialize Redis module with a pool connection
func Initialize(redisOptions conf.RedisSection) error {
	var err error
	Client, err = NewInstance(redisOptions)
	return err
}

// NewInstance returns an instance of Redis Client
func NewInstance(opt conf.RedisSection) (*redis.Client, error) {
	if !opt.SentinelReplication {
		return redis.NewClient(
			&redis.Options{
				Network:      opt.Network,
				Addr:         strings.Join([]string{opt.Host, strconv.FormatInt(int64(opt.Port), 10)}, ":"),
				Password:     opt.Pass,
				DB:           opt.Db,
				MaxRetries:   opt.MaxRetries,
				PoolSize:     opt.PoolSize,
				DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
				ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
				WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
			}), nil
	}

	if opt.SentinelMaster == "" {
		return nil, errors.New("Missing redis sentinel master name")
	}

	if opt.SentinelAddresses == "" {
		return nil, errors.New("Missing redis sentinels addresses")
	}

	addresses := strings.Split(opt.SentinelAddresses, ",")

	return redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    opt.SentinelMaster,
		SentinelAddrs: addresses,
		Password:      opt.Pass,
		DB:            opt.Db,
		MaxRetries:    opt.MaxRetries,
		PoolSize:      opt.PoolSize,
		DialTimeout:   time.Duration(opt.DialTimeout) * time.Second,
		ReadTimeout:   time.Duration(opt.ReadTimeout) * time.Second,
		WriteTimeout:  time.Duration(opt.WriteTimeout) * time.Second,
	}), nil
}
