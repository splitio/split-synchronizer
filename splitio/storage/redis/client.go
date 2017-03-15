// Package redis implements different kind of storages for split information
package redis

import (
	"strconv"
	"strings"

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
func Initialize(host string, port int, password string, db int) {
	Client = NewInstance(host, port, password, db)
}

// NewInstance returns an instance of Redis Client
func NewInstance(host string, port int,
	password string, db int) *redis.Client {

	redisClient := redis.NewClient(&redis.Options{
		Addr:     strings.Join([]string{host, strconv.FormatInt(int64(port), 10)}, ":"),
		Password: password,
		DB:       db,
	})

	return redisClient
}
