package storage

import (
	"errors"
	"github.com/stretchr/testify/mock"
	"testing"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"
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

type mockMySegmentsCache struct {
	updateErr error
}

func (m *mockMySegmentsCache) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet) error {
	return m.updateErr
}

func (m *mockMySegmentsCache) SegmentsForUser(key string) []string {
	return []string{}
}

func (m *mockMySegmentsCache) KeyCount() int {
	return 0
}

func TestSegmentStorageUpdateErrorHandling(t *testing.T) {
	t.Run("db error only", func(t *testing.T) {
		psm := &mocks.SegmentChangesCollectionMock{}
		dbErr := assert.AnError
		psm.On("Update", "segment1", (*set.ThreadUnsafeSet)(nil), (*set.ThreadUnsafeSet)(nil), int64(1)).Return(dbErr)

		mySegments := &mockMySegmentsCache{updateErr: nil}

		ss := ProxySegmentStorageImpl{
			logger:         logging.NewLogger(nil),
			db:             psm,
			mysegments:     mySegments,
			nameCountCache: observability.NewActiveSegmentTracker(10),
		}

		err := ss.Update("segment1", nil, nil, 1)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "errors updating db")
		assert.Contains(t, err.Error(), "errors updating cache: nil")
		psm.AssertExpectations(t)
	})
}

func TestSegmentSnapshotFromDiskWithBucketNotFoundError(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	segmentStorage := NewProxySegmentStorage(dbw, logger, true)

	namesAndCount := segmentStorage.NamesAndCount()
	assert.Empty(t, namesAndCount)
}

func TestSegmentSnapshotFromDiskWithSuccess(t *testing.T) {
	logger := logging.NewLogger(nil)

	dbw, err := persistent.NewBoltWrapper(persistent.BoltInMemoryMode, nil)
	assert.Nil(t, err)

	disk := persistent.NewSegmentChangesCollection(dbw, logger)
	disk.Update("segment1", set.NewSet("key1", "key2"), set.NewSet(), 10)

	segmentStorage := NewProxySegmentStorage(dbw, logger, true)

	namesAndCount := segmentStorage.NamesAndCount()
	assert.Contains(t, namesAndCount, "segment1")
	assert.Equal(t, 2, namesAndCount["segment1"])
}

type recordingSegmentWriter struct {
	calls    int
	name     string
	toAdd    []string
	toRemove []string
	cn       int64
}

func (r *recordingSegmentWriter) PutSegment(name string, toAdd, toRemove []string, cn int64) {
	r.calls++
	r.name = name
	r.toAdd = toAdd
	r.toRemove = toRemove
	r.cn = cn
}

func TestProxySegmentStorageForwardsToRedisWriter(t *testing.T) {
	toAdd := set.NewSet("k1", "k2")
	toRemove := set.NewSet("k3")

	t.Run("bolt succeeds", func(t *testing.T) {
		psm := &mocks.SegmentChangesCollectionMock{}
		psm.On("Update", "seg", mock.Anything, mock.Anything, int64(3)).Return(nil)
		ss := &ProxySegmentStorageImpl{
			logger:         logging.NewLogger(nil),
			db:             psm,
			mysegments:     optimized.NewMySegmentsCache(),
			nameCountCache: observability.NewActiveSegmentTracker(10),
		}
		w := &recordingSegmentWriter{}
		ss.WithRedisWriter(w)

		assert.Nil(t, ss.Update("seg", toAdd, toRemove, 3))
		assert.Equal(t, 1, w.calls)
		assert.Equal(t, "seg", w.name)
		assert.ElementsMatch(t, []string{"k1", "k2"}, w.toAdd)
		assert.ElementsMatch(t, []string{"k3"}, w.toRemove)
		assert.Equal(t, int64(3), w.cn)
	})

	t.Run("bolt fails", func(t *testing.T) {
		psm := &mocks.SegmentChangesCollectionMock{}
		psm.On("Update", "seg", mock.Anything, mock.Anything, int64(4)).Return(errors.New("bolt down"))
		ss := &ProxySegmentStorageImpl{
			logger:         logging.NewLogger(nil),
			db:             psm,
			mysegments:     optimized.NewMySegmentsCache(),
			nameCountCache: observability.NewActiveSegmentTracker(10),
		}
		w := &recordingSegmentWriter{}
		ss.WithRedisWriter(w)

		assert.NotNil(t, ss.Update("seg", toAdd, toRemove, 4))
		assert.Equal(t, 1, w.calls)
		assert.ElementsMatch(t, []string{"k1", "k2"}, w.toAdd)
		assert.ElementsMatch(t, []string{"k3"}, w.toRemove)
		assert.Equal(t, int64(4), w.cn)
	})
}
