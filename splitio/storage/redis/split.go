package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"

	"github.com/go-redis/redis"
)

// SplitStorageAdapter implements SplitStorage interface
type SplitStorageAdapter struct {
	*BaseStorageAdapter
	mutext *sync.RWMutex
}

// NewSplitStorageAdapter returns an instance of SplitStorageAdapter
func NewSplitStorageAdapter(clientInstance redis.UniversalClient, prefix string) *SplitStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := SplitStorageAdapter{adapter, &sync.RWMutex{}}
	return &client
}

func (r SplitStorageAdapter) save(key string, split api.SplitDTO) error {
	err := r.client.Set(r.splitNamespace(key), split, 0).Err()
	if err != nil {
		log.Error.Println("Error saving item", key, "in Redis:", err)
	} else {
		log.Verbose.Println("Item saved at key:", key)
	}

	return err
}

func (r SplitStorageAdapter) remove(key string) error {
	val, err := r.client.Del(r.splitNamespace(key)).Result()
	if err != nil {
		log.Error.Println("Error removing item", key, "in Redis:")
		return err
	}
	if val <= 0 {
		return errors.New("Split does not exist")
	}
	log.Verbose.Println("Split removed at key:", key)
	return nil
}

func getValues(split []byte) (string, string, error) {
	var tmpSplit map[string]interface{}
	err := json.Unmarshal(split, &tmpSplit)
	if err != nil {
		log.Error.Println("Split Values couldn't be fetched", err)
		return "", "", err
	}
	key := tmpSplit["name"].(string)
	trafficTypeName := tmpSplit["trafficTypeName"].(string)
	return key, trafficTypeName, nil
}

// Save an split object
func (r SplitStorageAdapter) Save(split interface{}) error {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	if _, ok := split.([]byte); !ok {
		return errors.New("Expecting []byte type, Invalid format given")
	}

	splitName, trafficType, err := getValues(split.([]byte))
	if err != nil {
		log.Error.Println("Split Name & TrafficType couldn't be fetched", err)
		return err
	}

	existing := r.getSplit(splitName)
	if existing != nil {
		// If it's an update, we decrement the traffic type count of the existing split,
		// and then add the updated one (as part of the normal flow), in case it's different.
		r.decr(existing.TrafficTypeName)
	}

	r.incr(trafficType)

	err = r.client.Set(r.splitNamespace(splitName), string(split.([]byte)), 0).Err()
	if err != nil {
		log.Error.Println("Error saving item", splitName, "in Redis:", err)
	} else {
		log.Verbose.Println("Item saved at key:", splitName)
	}

	return err

}

//Remove removes split item from redis
func (r SplitStorageAdapter) Remove(split interface{}) error {
	r.mutext.Lock()
	defer r.mutext.Unlock()

	if _, ok := split.([]byte); !ok {
		return errors.New("Expecting []byte type, Invalid format given")
	}

	splitName, trafficType, err := getValues(split.([]byte))
	if err != nil {
		log.Error.Println("Split Name & TrafficType couldn't be fetched", err)
		return err
	}

	existing := r.getSplit(splitName)
	if existing == nil {
		log.Info.Println("Split couldn't be fetched", splitName)
		return nil
	}
	r.decr(trafficType)
	return r.remove(splitName)
}

//RegisterSegment add the segment name into redis set
func (r SplitStorageAdapter) RegisterSegment(name string) error {
	err := r.client.SAdd(r.segmentsRegisteredNamespace(), name).Err()
	if err != nil {
		log.Debug.Println("Error saving segment", name, err)
	}
	return err
}

// SetChangeNumber sets the till value belong to segmentName
func (r SplitStorageAdapter) SetChangeNumber(changeNumber int64) error {
	return r.client.Set(r.splitsTillNamespace(), changeNumber, 0).Err()
}

// ChangeNumber gets the till value belong to segmentName
func (r SplitStorageAdapter) ChangeNumber() (int64, error) {
	return r.client.Get(r.splitsTillNamespace()).Int64()
}

// SplitsNames fetchs splits names from redis
func (r SplitStorageAdapter) SplitsNames() ([]string, error) {
	splitNames := r.client.Keys(r.splitNamespace("*"))
	err := splitNames.Err()
	if err != nil {
		log.Error.Println("Error fetching split names from Redis", err)
		return nil, err
	}

	rawNames := splitNames.Val()
	toReturn := make([]string, 0)
	for _, rawName := range rawNames {
		toReturn = append(toReturn, strings.Replace(rawName, r.splitNamespace(""), "", 1))
	}

	return toReturn, nil
}

// RawSplits return an slice with Split json representation
func (r SplitStorageAdapter) RawSplits() ([]string, error) {
	splitsNames, err := r.SplitsNames()
	if err != nil {
		return nil, err
	}

	toReturn := make([]string, 0)
	for _, splitName := range splitsNames {
		splitJSON, err := r.client.Get(r.splitNamespace(splitName)).Result()
		if err != nil {
			log.Error.Printf("Error fetching split from redis: %s\n", splitName)
			continue
		}
		toReturn = append(toReturn, splitJSON)
	}

	return toReturn, nil
}

// getSplit grabs split
func (r SplitStorageAdapter) getSplit(splitName string) *api.SplitDTO {
	raw, err := r.client.Get(r.splitNamespace(splitName)).Bytes()
	if err != nil {
		return nil
	}
	var splitDTO *api.SplitDTO
	if err := json.Unmarshal(raw, &splitDTO); err != nil {
		log.Error.Println(err)
		return nil
	}

	return splitDTO
}

// incr stores/increments trafficType in Redis
func (r SplitStorageAdapter) incr(trafficType string) error {
	trafficTypeToIncr := r.trafficTypeNamespace(trafficType)

	err := r.client.Incr(trafficTypeToIncr).Err()
	if err != nil {
		log.Error.Println(fmt.Sprintf("Error storing trafficType %s in redis", trafficType))
		log.Error.Println(err)
		return errors.New("Error incrementing trafficType")
	}
	return nil
}

// decr decrements trafficType count in Redis
func (r SplitStorageAdapter) decr(trafficType string) error {
	trafficTypeToDecr := r.trafficTypeNamespace(trafficType)
	if r.client.Decr(trafficTypeToDecr).Val() <= 0 {
		err := r.client.Del(trafficTypeToDecr).Err()
		if err != nil {
			log.Verbose.Println(fmt.Sprintf("Error removing trafficType %s in redis", trafficType))
		}
	}
	return nil
}
