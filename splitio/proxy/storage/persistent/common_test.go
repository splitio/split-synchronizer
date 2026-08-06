package persistent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangesItem(t *testing.T) {
	item := ChangesItem{
		ChangeNumber: 123,
		Name:         "test_split",
		Status:       "ACTIVE",
		JSON:         `{"name":"test_split","status":"ACTIVE"}`,
	}

	items := NewChangesItems(2)
	items.Append(item)
	items.Append(ChangesItem{
		ChangeNumber: 124,
		Name:         "another_split",
		Status:       "ARCHIVED",
		JSON:         `{"name":"another_split","status":"ARCHIVED"}`,
	})

	assert.Equal(t, 2, items.Len(), "Expected length to be 2")
	assert.Equal(t, "test_split", items.items[0].Name)
	assert.Equal(t, int64(123), items.items[0].ChangeNumber)
	assert.Equal(t, "ACTIVE", items.items[0].Status)
	assert.Equal(t, `{"name":"test_split","status":"ACTIVE"}`, items.items[0].JSON)
	assert.Equal(t, "another_split", items.items[1].Name)
	assert.Equal(t, int64(124), items.items[1].ChangeNumber)
	assert.Equal(t, "ARCHIVED", items.items[1].Status)
	assert.Equal(t, `{"name":"another_split","status":"ARCHIVED"}`, items.items[1].JSON)
	assert.True(t, items.Less(1, 0), "Expected item at index 1 to be less than item at index 0")
	items.Swap(0, 1)
	assert.Equal(t, "another_split", items.items[0].Name)
	assert.Equal(t, int64(124), items.items[0].ChangeNumber)
	assert.Equal(t, "ARCHIVED", items.items[0].Status)
	assert.Equal(t, `{"name":"another_split","status":"ARCHIVED"}`, items.items[0].JSON)
	assert.Equal(t, "test_split", items.items[1].Name)
	assert.Equal(t, int64(123), items.items[1].ChangeNumber)
	assert.Equal(t, "ACTIVE", items.items[1].Status)
	assert.Equal(t, `{"name":"test_split","status":"ACTIVE"}`, items.items[1].JSON)
}
