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
		storage := NewOverrideStorage(mockedStorage)
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

		storage := NewOverrideStorage(mockedStorage)
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

		storage := NewOverrideStorage(mockedStorage)
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
		// Remove the override
		storage.RemoveOverrideFF("ff1")
		// Fetch the same flag again to ensure it is not cached
		cachedFF = storage.FF("ff1")
		assert.Nil(t, cachedFF)

		mockedStorage.AssertExpectations(t)
	})
}
