package redis

import (
	"errors"
	"strings"

	"github.com/go-redis/redis"
)

const ErrorHashNotPresent = "hash-not-present"

const clearAllSCriptTemplate = `
	local toDelete = redis.call('KEYS', '{KEY_NAMESPACE}*')
	local count = 0
	for _, key in ipairs(toDelete) do
	    redis.call('DEL', key)
	    count = count + 1
	end
	return count
`

// MiscStorageAdapter provides methods to handle the synchronizer's initialization procedure
type MiscStorageAdapter struct {
	*BaseStorageAdapter
}

func (b BaseStorageAdapter) GetApikeyHash() (string, error) {
	res := b.client.Get(b.hashNamespace())
	if res.Err() != nil && res.Err().Error() == "redis: nil" {
		return "", errors.New(ErrorHashNotPresent)
	}
	return res.Val(), res.Err()
}

func (b BaseStorageAdapter) SetApikeyHash(newApikeyHash string) error {
	res := b.client.Set(b.hashNamespace(), newApikeyHash, 0)
	return res.Err()
}

func (b BaseStorageAdapter) ClearAll() error {
	luaCMD := strings.Replace(clearAllSCriptTemplate, "{KEY_NAMESPACE}", b.prefixAdapter.baseNamespace(), 1)
	cmd := b.client.Eval(luaCMD, nil, 0)
	return cmd.Err()
}

func NewMiscStorageAdapter(clientInstance redis.UniversalClient, prefix string) *MiscStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := MiscStorageAdapter{adapter}
	return &client
}
