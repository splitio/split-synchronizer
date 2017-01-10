// Package storage implements different kind of storages for split information
package storage

import (
	"strconv"
	"strings"

	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"

	redis "gopkg.in/redis.v5"
)

// RedisSplitStorageAdapter interface defines the split data storage
type RedisSplitStorageAdapter struct {
	client *redis.Client
}

// NewRedisSplitStorageAdapter implements a Redis client
func NewRedisSplitStorageAdapter(host string, port int, password string, db int) *RedisSplitStorageAdapter {

	redisClient := redis.NewClient(&redis.Options{
		Addr:     strings.Join([]string{host, strconv.FormatInt(int64(port), 10)}, ":"),
		Password: password,
		DB:       db,
	})

	client := RedisSplitStorageAdapter{client: redisClient}
	return &client
}

// Save an split in redis cache
func (r RedisSplitStorageAdapter) Save(key string, split interface{}) error {
	err := r.client.Set(key, split, 0).Err()
	if errors.IsError(err) {
		log.Error.Println("Error saving item in Redis ", err)
	} else {
		log.Verbose.Println("Item saved at key: ", key)
	}

	return err
}
