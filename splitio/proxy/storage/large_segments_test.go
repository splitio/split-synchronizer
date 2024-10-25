package storage

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

func sortedKeys(count int, shared *string) []string {
	keys := make([]string, 0, count)
	for i := 0; i < count; i++ {
		keys = append(keys, uuid.New().String())
	}

	if shared != nil {
		keys = append(keys, *shared)
	}

	sort.Strings(keys)
	return keys
}

func TestLatgeSegmentStorage(t *testing.T) {
	storage := NewLargeSegmentsStorage(logging.NewLogger(nil))

	keys1 := sortedKeys(10000, nil)
	storage.Update("ls_test_1", keys1)

	sharedKey := &keys1[5000]
	keys2 := sortedKeys(20000, sharedKey)
	storage.Update("ls_test_2", keys2)

	keys3 := sortedKeys(30000, sharedKey)
	storage.Update("ls_test_3", keys3)

	assert.Equal(t, 3, storage.Count())

	result := storage.LargeSegmentsForUser(*sharedKey)
	sort.Strings(result)
	assert.Equal(t, []string{"ls_test_1", "ls_test_2", "ls_test_3"}, result)

	result = storage.LargeSegmentsForUser(keys1[100])
	assert.Equal(t, []string{"ls_test_1"}, result)

	result = storage.LargeSegmentsForUser(keys2[100])
	assert.Equal(t, []string{"ls_test_2"}, result)

	result = storage.LargeSegmentsForUser(keys3[100])
	assert.Equal(t, []string{"ls_test_3"}, result)

	result = storage.LargeSegmentsForUser("mauro-test")
	assert.Equal(t, []string{}, result)
}
