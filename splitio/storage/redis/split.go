package redis

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"

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

// Save an split object
func (r SplitStorageAdapter) Save(split interface{}) error {
	if splitDto, ok := split.(api.SplitDTO); ok {
		return r.save(splitDto.Name, splitDto)
	}
	message := fmt.Sprintf("Invalid parameter type, SplitDTO is expected but %s found", reflect.TypeOf(split))
	log.Error.Println(message)
	return errors.New(message)
}

//Remove removes split item from redis
func (r SplitStorageAdapter) Remove(split interface{}) error {
	if splitDto, ok := split.(api.SplitDTO); ok {
		return r.remove(splitDto.Name)
	}
	message := fmt.Sprintf("Invalid parameter type, SplitDTO is expected but %s found", reflect.TypeOf(split))
	log.Error.Println(message)
	return errors.New(message)
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
