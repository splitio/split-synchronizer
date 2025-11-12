package observability

import (
	"errors"
	"testing"

	"github.com/splitio/go-split-commons/v8/dtos"
	"github.com/splitio/go-split-commons/v8/storage/mocks"
	"github.com/splitio/go-split-commons/v8/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
)

func TestSplitWrapper(t *testing.T) {
	st := &extMockSplitStorage{
		&mocks.MockSplitStorage{
			SplitNamesCall: func() []string {
				return []string{"split1", "split2"}
			},
			UpdateCall: func([]dtos.SplitDTO, []dtos.SplitDTO, int64) {
				t.Error("should not be called")
			},
		},
		nil,
	}

	observer, _ := NewObservableSplitStorage(st, logging.NewLogger(nil))
	if c := observer.Count(); c != 2 {
		t.Error("count sohuld be 2. Is ", c)
	}

	// split4 should fail to be updated
	st.UpdateWithErrorsCall = func(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) error {
		return &redis.UpdateError{
			FailedToAdd: map[string]error{"split4": errors.New("something")},
		}
	}
	observer.Update([]dtos.SplitDTO{{Name: "split3"}, {Name: "split4"}}, []dtos.SplitDTO{}, 1)
	if _, ok := observer.active.activeSplitMap["split3"]; !ok {
		t.Error("split3 should be cached")
	}
	if _, ok := observer.active.activeSplitMap["split4"]; ok {
		t.Error("split4 should not be cached")
	}
}

var errSome = errors.New("some random error")

func TestActiveSplitTracker(t *testing.T) {
	trk := newActiveSplitTracker(10)
	trk.update([]string{"split1", "split2"}, []string{"nonexistant"})
	n := trk.names()
	if len(n) != 2 || trk.count() != 2 {
		t.Error("there should be 2 elements")
	}

	trk.update(nil, []string{"split2"})
	if trk.names()[0] != "split1" {
		t.Error("there should be only one element 'split1'")
	}

	trk.update(nil, []string{"split1"})
	if len(trk.names()) != 0 || trk.count() != 0 {
		t.Error("there should be 0 items")
	}
}

func TestFilterFailed(t *testing.T) {
	splits := []dtos.SplitDTO{{Name: "split1"}, {Name: "split2"}, {Name: "split3"}, {Name: "split4"}}
	failed := map[string]error{
		"split2": errSome,
		"split4": errSome,
	}

	withoutFailed := filterFailed(splits, failed)
	if l := len(withoutFailed); l != 2 {
		t.Error("there should be 2 items after removing the failed ones. Have: ", l)
		t.Error(withoutFailed)
	}

	if n := withoutFailed[0].Name; n != "split1" {
		t.Error("first one should be 'split1'. is: ", n)
	}
	if n := withoutFailed[1].Name; n != "split3" {
		t.Error("first one should be 'split1'. is: ", n)
	}
}

type extMockSplitStorage struct {
	*mocks.MockSplitStorage
	UpdateWithErrorsCall func([]dtos.SplitDTO, []dtos.SplitDTO, int64) error
}

func (e *extMockSplitStorage) UpdateWithErrors(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) error {
	return e.UpdateWithErrorsCall(toAdd, toRemove, cn)
}

var _ extendedSplitStorage = (*extMockSplitStorage)(nil)
