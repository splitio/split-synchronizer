package redis

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"

	redis "gopkg.in/redis.v5"
)

// SplitStorageAdapter implements SplitStorage interface
type SplitStorageAdapter struct {
	*BaseStorageAdapter
}

// NewSplitStorageAdapter returns an instance of SplitStorageAdapter
func NewSplitStorageAdapter(clientInstance *redis.Client, prefix string) *SplitStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := SplitStorageAdapter{adapter}
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
	err := r.client.Del(r.splitNamespace(key)).Err()
	if err != nil {
		log.Error.Println("Error removing item", key, "in Redis:", err)
	} else {
		log.Verbose.Println("Item removed at key:", key)
	}

	return err
}

func getKey(split []byte) (string, error) {
	var tmpSplit map[string]interface{}
	err := json.Unmarshal(split, &tmpSplit)
	if err != nil {
		log.Error.Println("Split Name couldn't be fetched", err)
		return "", err
	}
	key := tmpSplit["name"].(string)
	return key, nil
}

// Save an split object
func (r SplitStorageAdapter) Save(split interface{}) error {

	if _, ok := split.([]byte); !ok {
		return errors.New("Expecting []byte type, Invalid format given")
	}

	key, err := getKey(split.([]byte))
	if err != nil {
		log.Error.Println("Split Name couldn't be fetched", err)
		return err
	}

	err = r.client.Set(r.splitNamespace(key), string(split.([]byte)), 0).Err()
	if err != nil {
		log.Error.Println("Error saving item", key, "in Redis:", err)
	} else {
		log.Verbose.Println("Item saved at key:", key)
	}

	return err

}

//Remove removes split item from redis
func (r SplitStorageAdapter) Remove(split interface{}) error {

	if _, ok := split.([]byte); !ok {
		return errors.New("Expecting []byte type, Invalid format given")
	}

	key, err := getKey(split.([]byte))
	if err != nil {
		log.Error.Println("Split Name couldn't be fetched", err)
		return err
	}

	return r.remove(key)
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
