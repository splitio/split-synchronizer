package redis

import (
	"github.com/go-redis/redis"
	"strings"
)

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
	return res.Val(), res.Err()
}

func (b BaseStorageAdapter) SetApikeyHash(newApikeyHash string) error {
	res := b.client.Set(b.hashNamespace(), newApikeyHash, 0)
	return res.Err()
}

func (b BaseStorageAdapter) ClearAll() error {
	luaCMD := strings.Replace(clearAllSCriptTemplate, "{KEY_NAMESPACE}", b.prefix+"."+"SPLITIO", 1)
	cmd := b.client.Eval(luaCMD, nil, 0)
	return cmd.Err()
}

func NewMiscStorageAdapter(clientInstance redis.UniversalClient, prefix string) *MiscStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := MiscStorageAdapter{adapter}
	return &client
}
