package redis

import (
	"encoding/json"
	"math"
	"sync"

	"github.com/go-redis/redis"
	"github.com/splitio/go-toolkit/queuecache"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

var elMutex = &sync.Mutex{}

// MaxAccumulatedSize is the maximum number of bytes to be fetched from cache before posting to the backend
const MaxAccumulatedSize = 5 * 1024 * 1024

// MaxEventSize is the maximum allowed event size
const MaxEventSize = 32 * 1024

// EventStorageAdapter implements EventStorage interface
type EventStorageAdapter struct {
	*BaseStorageAdapter
	cache queuecache.InMemoryQueueCacheOverlay
}

// NewEventStorageAdapter returns an instance of EventStorageAdapter
func NewEventStorageAdapter(clientInstance redis.UniversalClient, prefix string) *EventStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}

	refillFunc := func(count int) ([]interface{}, error) {
		elMutex.Lock()
		defer elMutex.Unlock()
		lrange := adapter.client.LRange(adapter.eventsListNamespace(), 0, int64(count-1))
		if lrange.Err() != nil {
			log.Error.Println("Fetching events", lrange.Err().Error())
			return nil, lrange.Err()
		}
		totalFetchedEvents := len(lrange.Val())

		idxFrom := count
		if totalFetchedEvents < count {
			idxFrom = totalFetchedEvents
		}

		res := adapter.client.LTrim(adapter.eventsListNamespace(), int64(idxFrom), -1)
		if res.Err() != nil {
			log.Error.Println("Trim events", res.Err().Error())
			return nil, res.Err()
		}

		toReturn := make([]interface{}, len(lrange.Val()))
		for index, item := range lrange.Val() {
			toReturn[index] = item
		}
		return toReturn, nil
	}

	client := EventStorageAdapter{
		BaseStorageAdapter: adapter,
		cache:              *queuecache.New(10000, refillFunc),
	}
	return &client
}

// PopN returns elements given by LRANGE 0 items and perform a LTRIM items -1
func (r *EventStorageAdapter) PopN(n int64) ([]api.RedisStoredEventDTO, error) {
	toReturn := make([]api.RedisStoredEventDTO, n)
	var err error
	fetchedCount := 0
	accumulatedSize := 0
	writeIndex := 0
	for int64(fetchedCount) < n && accumulatedSize < MaxAccumulatedSize && err == nil {
		numberOfItemsToFetch := int(math.Min(
			float64((MaxAccumulatedSize-accumulatedSize)/MaxEventSize),
			float64(n-int64(fetchedCount)),
		))
		elems, err := r.cache.Fetch(numberOfItemsToFetch)
		if err != nil {
			log.Error.Println("Error fetching events", err.Error())
			break
		}

		for _, elem := range elems {
			asStr, ok := elem.(string)
			if !ok {
				log.Error.Println("Error type-asserting event as string", err.Error())
				continue
			}

			storedEventDTO := api.RedisStoredEventDTO{}
			err = json.Unmarshal([]byte(asStr), &storedEventDTO)
			if err != nil {
				log.Error.Println("Error decoding event JSON", err.Error())
				continue
			}
			accumulatedSize += storedEventDTO.Event.Size()
			toReturn[writeIndex] = storedEventDTO
			writeIndex++
		}
		fetchedCount += len(elems)
	}
	return toReturn[0:writeIndex], nil
}

// Drop drops events from queue
func (r *EventStorageAdapter) Drop(size *int64) error {
	elMutex.Lock()
	defer elMutex.Unlock()
	return r.BaseStorageAdapter.Drop(r.eventsListNamespace(), size)
}

// Size returns the size of the impressions queue
func (r *EventStorageAdapter) Size() int64 {
	return r.BaseStorageAdapter.Size(r.eventsListNamespace()) + int64(r.cache.Count())
}
