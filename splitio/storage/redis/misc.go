package redis

import (
	"strings"

	"github.com/go-redis/redis"
)

// MiscStorageAdapter provides methods to handle the synchronizer's initialization procedure
type MiscStorageAdapter struct {
	*BaseStorageAdapter
}

func (b BaseStorageAdapter) GetApikeyHash() (string, error) {
	res := b.client.Get(b.hashNamespace())
	return res.String(), res.Err()
}

func (b BaseStorageAdapter) SetApikeyHash(newApikeyHash string) error {
	res := b.client.Set(b.hashNamespace(), newApikeyHash, 0)
	return res.Err()
}

func (b BaseStorageAdapter) ClearAll() error {
	luaCMD := strings.Replace(clearAllSCriptTemplate, "{KEY_NAMESPACE}", b.prefix+"."+"SPLITIO", 0)
	cmd := b.client.Eval(luaCMD, nil, 0)
	return cmd.Err()
}

func NewMiscStorageAdapter(clientInstance redis.UniversalClient, prefix string) *MiscStorageAdapter {
	prefixAdapter := &prefixAdapter{prefix: prefix}
	adapter := &BaseStorageAdapter{prefixAdapter, clientInstance}
	client := MiscStorageAdapter{adapter}
	return &client
}
