package redis

import (
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
func Initialize(redisOptions conf.RedisSection) {
	Client = NewInstance(redisOptions)
}

// NewInstance returns an instance of Redis Client
func NewInstance(opt conf.RedisSection) *redis.Client {

	redisClient := redis.NewClient(&redis.Options{
		Network:      opt.Network,
		Addr:         strings.Join([]string{opt.Host, strconv.FormatInt(int64(opt.Port), 10)}, ":"),
		Password:     opt.Pass,
		DB:           opt.Db,
		MaxRetries:   opt.MaxRetries,
		PoolSize:     opt.PoolSize,
		DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second})

	return redisClient
}
