package storage

import (
	"reflect"
	"testing"

	"github.com/splitio/go-split-commons/v4/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestSegmentWrapper(t *testing.T) {
	st := &extMockSegmentStorage{
		MockSegmentStorage: &mocks.MockSegmentStorage{
			UpdateCall: func(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, till int64) error {
				t.Error("should not be called!")
				return nil
			},
		},
		SizeCall: func(name string) (int, error) {
			switch name {
			case "segment1":
				return 10, nil
			case "segment2":
				return 20, nil
			}
			return 0, nil
		},
		UpdateWithSummaryCall: func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) {
			return 0, 0, nil
		},
	}

	splitStorage := &mocks.MockSplitStorage{
		SegmentNamesCall: func() *set.ThreadUnsafeSet { return set.NewSet("segment1", "segment2") },
	}

	observer := NewObservableSegmentStorage(logging.NewLogger(nil), splitStorage, st)

	expected := map[string]int{
		"segment1": 10,
		"segment2": 20,
	}

	if !reflect.DeepEqual(expected, observer.NamesAndCount()) {
		t.Error("names and count doesn't match expected")
	}

	expected["segment3"] = 3
	st.UpdateWithSummaryCall = func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) { return 3, 0, nil }
	observer.Update("segment3", set.NewSet("k1", "k2", "k3"), set.NewSet(), 123)
	if !reflect.DeepEqual(expected, observer.NamesAndCount()) {
		t.Error("names and count doesn't match expected")
	}

	delete(expected, "segment3")
	st.UpdateWithSummaryCall = func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) { return 0, 3, nil }
	observer.Update("segment3", set.NewSet(), set.NewSet("k1", "k2", "k3"), 123)
	if !reflect.DeepEqual(expected, observer.NamesAndCount()) {
		t.Error("names and count doesn't match expected")
	}

	expected["segment2"] = 22
	st.UpdateWithSummaryCall = func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) { return 3, 1, nil }
	observer.Update("segment2", set.NewSet("k1", "k2", "k3"), set.NewSet("k4"), 123)
	if !reflect.DeepEqual(expected, observer.NamesAndCount()) {
		t.Error("names and count doesn't match expected")
	}

	delete(expected, "segment1")
	st.UpdateWithSummaryCall = func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) { return 0, 10, nil }
	observer.Update("segment1", set.NewSet(), set.NewSet("k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10"), 123)
	if !reflect.DeepEqual(expected, observer.NamesAndCount()) {
		t.Error("names and count doesn't match expected")
	}
}

func TestActiveSegmentTracker(t *testing.T) {

	expected := map[string]int{
		"segment1": 50,
		"segment2": 40,
		"segment3": 30,
	}

	trk := newActiveSegmentTracker(10)
	trk.update("segment1", 50, 0)
	trk.update("segment2", 40, 0)
	trk.update("segment3", 30, 0)
	if trk.count() != 3 {
		t.Error("there should be 3 segments cached")
	}

	if !reflect.DeepEqual(expected, trk.namesAndCount()) {
		t.Error("current status doens't match expected")
	}

	trk.update("segment4", 0, 300)
	if !reflect.DeepEqual(expected, trk.namesAndCount()) {
		t.Error("current status doens't match expected")
	}

	trk.update("segment1", 0, 49)
	expected["segment1"] = 1
	if !reflect.DeepEqual(expected, trk.namesAndCount()) {
		t.Error("current status doens't match expected")
	}

	trk.update("segment1", 0, 1)
	delete(expected, "segment1")
	if !reflect.DeepEqual(expected, trk.namesAndCount()) {
		t.Error("current status doens't match expected")
	}

	if trk.count() != 2 {
		t.Error("there should be 2 elements now")
	}

}

type extMockSegmentStorage struct {
	*mocks.MockSegmentStorage
	UpdateWithSummaryCall func(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error)
	SizeCall              func(string) (int, error)
}

func (e *extMockSegmentStorage) UpdateWithSummary(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, till int64) (added int, removed int, err error) {
	return e.UpdateWithSummaryCall(name, toAdd, toRemove, till)
}

func (e *extMockSegmentStorage) Size(name string) (int, error) {
	return e.SizeCall(name)
}
