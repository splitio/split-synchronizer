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
	defer trafficMutex.Unlock()
	trafficTypeToDecr := t.trafficTypeNamespace() + "." + trafficType

	trafficMutex.Lock()
	v, _ := t.client.Get(trafficTypeToDecr).Int()
	if v > 0 {
		err := t.client.Decr(trafficTypeToDecr).Err()
		if err != nil {
			log.Error.Println(fmt.Sprintf("Error storing trafficType %s in redis", trafficType))
			log.Error.Println(err)
			return errors.New("Error decrementing trafficType")
		}
	}
	return nil
}
