package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"

	"github.com/splitio/go-split-commons/v6/dtos"

	"github.com/stretchr/testify/assert"
)

func TestFeatureFlag(t *testing.T) {
	t.Run("OverrideFF FlagNotFound", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"nonexistent"}).Return(nil).Once()
		storage := NewOverrideStorage(mockedStorage, nil)
		_, err := storage.OverrideFF("nonexistent", nil, nil)
		assert.NotNil(t, err)
		assert.ErrorAs(t, err, &ErrFeatureFlagNotFound)

		mockedStorage.AssertExpectations(t)
	})

	t.Run("OverrideFF UpdateFlag", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"ff1"}).Return(map[string]*dtos.SplitDTO{
			"ff1": {
				Name:             "ff1",
				Killed:           false,
				DefaultTreatment: "on",
			},
		}).Once()

		storage := NewOverrideStorage(mockedStorage, nil)
		killed := true
		defaultTreatment := "off"
		ff, err := storage.OverrideFF("ff1", &killed, &defaultTreatment)
		assert.Nil(t, err)
		assert.Equal(t, ff.Name, "ff1")
		assert.Equal(t, ff.Killed, killed)
		assert.Equal(t, ff.DefaultTreatment, defaultTreatment)

		mockedStorage.AssertExpectations(t)
	})
	t.Run("Integration", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"ff1"}).Return(map[string]*dtos.SplitDTO{
			"ff1": {
				Name:             "ff1",
				Killed:           false,
				DefaultTreatment: "on",
			},
		}).Once()

		storage := NewOverrideStorage(mockedStorage, nil)
		killed := true
		defaultTreatment := "off"
		// Override the feature flag
		ff, err := storage.OverrideFF("ff1", &killed, &defaultTreatment)
		assert.Nil(t, err)
		assert.Equal(t, ff.Name, ff.Name)
		assert.Equal(t, ff.Killed, true)
		assert.Equal(t, ff.DefaultTreatment, "off")
		// Fetch the same flag again to ensure it is cached
		cachedFF := storage.FF("ff1")
		assert.NotNil(t, cachedFF)
		assert.Equal(t, cachedFF.Name, ff.Name)
		assert.Equal(t, cachedFF.Killed, true)
		assert.Equal(t, cachedFF.DefaultTreatment, "off")
		// Get all overrides to ensure the override is present
		overrides := storage.GetOverrides()
		assert.Len(t, overrides, 1)
		assert.Equal(t, overrides["ff1"].Name, "ff1")
		assert.Equal(t, overrides["ff1"].Killed, true)
		assert.Equal(t, overrides["ff1"].DefaultTreatment, "off")
		// Remove the override
		storage.RemoveOverrideFF("ff1")
		// Fetch the same flag again to ensure it is not cached
		cachedFF = storage.FF("ff1")
		assert.Nil(t, cachedFF)

		mockedStorage.AssertExpectations(t)
	})
}

// TestSegment tests the segment override functionality
func TestSegment(t *testing.T) {
	t.Run("OverrideSegment Integration", func(t *testing.T) {
		storage := NewOverrideStorage(nil)
		segment1u1 := storage.OverrideSegment("user1", "segment1", "add")
		segment2u1 := storage.OverrideSegment("user1", "segment2", "remove")
		segment2u2 := storage.OverrideSegment("user2", "segment2", "add")

		assert.Equal(t, SegmentOverride{Operation: "add"}, segment1u1)
		assert.Equal(t, SegmentOverride{Operation: "remove"}, segment2u1)
		assert.Equal(t, SegmentOverride{Operation: "add"}, segment2u2)

		overridesu1 := storage.Segment("user1")
		assert.Equal(t, SegmentOverride{Operation: "add"}, overridesu1["segment1"])
		assert.Equal(t, SegmentOverride{Operation: "remove"}, overridesu1["segment2"])
		overridesu2 := storage.Segment("user2")
		assert.Equal(t, SegmentOverride{Operation: "add"}, overridesu2["segment2"])

		overridesForUs1 := storage.GetOverridesForSegment()
		assert.Len(t, overridesForUs1, 2)
		assert.Contains(t, overridesForUs1["user1"], PerKey{Operation: "add", Key: "segment1"})
		assert.Contains(t, overridesForUs1["user1"], PerKey{Operation: "remove", Key: "segment2"})
		assert.Contains(t, overridesForUs1["user2"], PerKey{Operation: "add", Key: "segment2"})

		storage.RemoveOverrideSegment("user1", "segment1")
		overridesu1 = storage.Segment("user1")
		assert.Len(t, overridesu1, 1)
		assert.Equal(t, SegmentOverride{Operation: "remove"}, overridesu1["segment2"])
	})
}
