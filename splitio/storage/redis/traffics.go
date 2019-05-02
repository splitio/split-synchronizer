package redis

import (
	"errors"
	"fmt"
	"sync"

	"github.com/go-redis/redis"
	"github.com/splitio/split-synchronizer/log"
)

var trafficMutex sync.Mutex

// TrafficTypeStorageAdapter implements TrafficTypeStorage interface
type TrafficTypeStorageAdapter struct {
	*BaseStorageAdapter
}

// NewTrafficTypeStorageAdapter returns an instance of TrafficTypeStorageAdapter
func NewTrafficTypeStorageAdapter(clientInstance redis.UniversalClient, prefix string) *TrafficTypeStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := TrafficTypeStorageAdapter{BaseStorageAdapter: adapter}
	return &client
}

// Incr stores/increments trafficType in Redis
func (t TrafficTypeStorageAdapter) Incr(trafficType string) error {
	trafficTypeToIncr := t.trafficTypeNamespace() + "." + trafficType

	err := t.client.Incr(trafficTypeToIncr).Err()
	if err != nil {
		log.Error.Println(fmt.Sprintf("Error storing trafficType %s in redis", trafficType))
		log.Error.Println(err)
		return errors.New("Error incrementing trafficType")
	}
	return nil
}

// Decr decrements trafficType count in Redis
func (t TrafficTypeStorageAdapter) Decr(trafficType string) error {
	trafficMutex.Lock()
	defer trafficMutex.Unlock()
	trafficTypeToDecr := t.trafficTypeNamespace() + "." + trafficType
	v, _ := t.client.Get(trafficTypeToDecr).Int()
	if v > 0 {
		err := t.client.Decr(trafficTypeToDecr).Err()
		if err != nil {
			log.Error.Println(fmt.Sprintf("Error storing trafficType %s in redis", trafficType))
			log.Error.Println(err)
			return errors.New("Error decrementing trafficType")
		}
	} else {
		err := t.client.Del(trafficTypeToDecr).Err()
		if err != nil {
			log.Verbose.Println(fmt.Sprintf("Error removing trafficType %s in redis", trafficType))
		}
	}
	return nil
}

// Clean erase all the trafficTypes in Redis
func (t TrafficTypeStorageAdapter) Clean() error {
	trafficMutex.Lock()
	defer trafficMutex.Unlock()

	trafficTypes, err := t.client.Keys(t.trafficTypeNamespace() + "*").Result()
	if err != nil {
		log.Error.Println("Error fetching trafficTypes in redis")
		log.Error.Println(err)
		return errors.New("Error fetching trafficTypes in redis")
	}
	if len(trafficTypes) > 0 {
		err = t.client.Del(trafficTypes...).Err()
		if err != nil {
			log.Error.Println("Error cleaning trafficTypes in redis")
			log.Error.Println(err)
			return errors.New("Error cleaning trafficTypes in redis")
		}
	}
	return nil
}
