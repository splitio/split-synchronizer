package storage

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/optimized"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent/mocks"
	"github.com/stretchr/testify/assert"
)

func TestSegmentStorage(t *testing.T) {

	psm := &mocks.SegmentChangesCollectionMock{}
	psm.On("Fetch", "some").Return(&persistent.SegmentChangesItem{
		Name: "some",
		Keys: map[string]persistent.SegmentKey{
			"k1": {Name: "k1", ChangeNumber: 1, Removed: false},
			"k2": {Name: "k2", ChangeNumber: 1, Removed: true},
			"k3": {Name: "k3", ChangeNumber: 2, Removed: false},
			"k4": {Name: "k4", ChangeNumber: 2, Removed: true},
			"k5": {Name: "k5", ChangeNumber: 3, Removed: false},
			"k6": {Name: "k6", ChangeNumber: 3, Removed: true},
			"k7": {Name: "k7", ChangeNumber: 4, Removed: false},
		},
	}, nil)

	ss := ProxySegmentStorageImpl{
		logger:     logging.NewLogger(nil),
		db:         psm,
		mysegments: optimized.NewMySegmentsCache(),
	}

	changes, err := ss.ChangesSince("some", -1)
	assert.Nil(t, err)
	assert.Equal(t, "some", changes.Name)
	assert.ElementsMatch(t, []string{"k1", "k3", "k5", "k7"}, changes.Added)
	assert.ElementsMatch(t, []string{}, changes.Removed)
	assert.Equal(t, int64(-1), changes.Since)
	assert.Equal(t, int64(4), changes.Till)

	changes, err = ss.ChangesSince("some", 1)
	assert.Nil(t, err)
	assert.Equal(t, "some", changes.Name)
	assert.ElementsMatch(t, []string{"k3", "k5", "k7"}, changes.Added)
	assert.ElementsMatch(t, []string{"k4", "k6"}, changes.Removed)
	assert.Equal(t, int64(1), changes.Since)
	assert.Equal(t, int64(4), changes.Till)

	changes, err = ss.ChangesSince("some", 2)
	assert.Nil(t, err)
	assert.Equal(t, "some", changes.Name)
	assert.ElementsMatch(t, []string{"k5", "k7"}, changes.Added)
	assert.ElementsMatch(t, []string{"k6"}, changes.Removed)
	assert.Equal(t, int64(2), changes.Since)
	assert.Equal(t, int64(4), changes.Till)

	changes, err = ss.ChangesSince("some", 3)
	assert.Nil(t, err)
	assert.Equal(t, "some", changes.Name)
	assert.ElementsMatch(t, []string{"k7"}, changes.Added)
	assert.ElementsMatch(t, []string{}, changes.Removed)
	assert.Equal(t, int64(3), changes.Since)
	assert.Equal(t, int64(4), changes.Till)

	changes, err = ss.ChangesSince("some", 4)
	assert.Nil(t, err)
	assert.Equal(t, "some", changes.Name)
	assert.ElementsMatch(t, []string{}, changes.Added)
	assert.ElementsMatch(t, []string{}, changes.Removed)
	assert.Equal(t, int64(4), changes.Since)
	assert.Equal(t, int64(4), changes.Till)

}
