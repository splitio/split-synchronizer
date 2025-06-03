package storage

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/mocks"

	"github.com/splitio/go-split-commons/v6/dtos"

	"github.com/stretchr/testify/assert"
)

func TestFeatureFlag(t *testing.T) {
	t.Run("FeatureFlag FlagNotFound", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"nonexistent"}).Return(nil).Once()
		storage := NewOverrideStorage(mockedStorage)
		_, err := storage.FeatureFlag("nonexistent", nil, nil)
		assert.NotNil(t, err)
		assert.ErrorAs(t, err, &ErrFeatureFlagNotFound)

		mockedStorage.AssertExpectations(t)
	})

	t.Run("FeatureFlag UpdateFlag", func(t *testing.T) {
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
		ff, err := storage.FeatureFlag("ff1", &killed, &defaultTreatment)
		assert.Nil(t, err)
		assert.Equal(t, ff.Name, "ff1")
		assert.Equal(t, ff.Killed, killed)
		assert.Equal(t, ff.DefaultTreatment, defaultTreatment)

		mockedStorage.AssertExpectations(t)
	})

	t.Run("DeleteFlag FlagNotFound", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"nonexistent"}).Return(nil).Once()
		storage := NewOverrideStorage(mockedStorage)
		_, err := storage.DeleteFeatureFlag("nonexistent")
		assert.NotNil(t, err)
		assert.ErrorAs(t, err, &ErrFeatureFlagNotFound)

		mockedStorage.AssertExpectations(t)
	})

	t.Run("DeleteFlag Integration", func(t *testing.T) {
		mockedStorage := &mocks.ProxySplitStorageMock{}
		mockedStorage.On("FetchMany", []string{"ff1"}).Return(map[string]*dtos.SplitDTO{
			"ff1": {
				Name:             "ff1",
				Killed:           false,
				DefaultTreatment: "on",
			},
		}).Twice()

		storage := NewOverrideStorage(mockedStorage)
		killed := true
		defaultTreatment := "off"
		ff, err := storage.FeatureFlag("ff1", &killed, &defaultTreatment)
		assert.Nil(t, err)
		assert.Equal(t, ff.Name, ff.Name)
		assert.Equal(t, ff.Killed, killed)
		assert.Equal(t, ff.DefaultTreatment, defaultTreatment)

		deletedFF, err := storage.DeleteFeatureFlag("ff1")
		assert.Nil(t, err)
		assert.Equal(t, deletedFF.Name, ff.Name)
		assert.Equal(t, deletedFF.Killed, ff.Killed)
		assert.Equal(t, deletedFF.DefaultTreatment, ff.DefaultTreatment)

		mockedStorage.AssertExpectations(t)
	})
}
