package optimized

import (
	"testing"

	"github.com/splitio/go-split-commons/v4/dtos"
)

func stringSlicesEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func validateChanges(t *testing.T, c *ChangeSummary, expectedAdded []string, expectedRemoved []string) {
	t.Helper()

	if len(c.Updated) != len(expectedAdded) || len(c.Removed) != len(expectedRemoved) {
		t.Error("incorrect changes lengths")
	}

	for _, added := range expectedAdded {
		if _, ok := c.Updated[added]; !ok {
			t.Errorf("key %s should be in `updated` and isnt.", added)
		}
	}

	for _, removed := range expectedRemoved {
		if _, ok := c.Removed[removed]; !ok {
			t.Errorf("key %s should be in `removed` and isnt.", removed)
		}
	}
}

func TestSplitChangesSummary(t *testing.T) {
	summaries := NewSplitChangesSummaries()
	changesM1, cnM1, err := summaries.FetchSince(-1)
	if err != nil {
		t.Error(err)
	}
	if cnM1 != -1 {
		t.Error("cn should be -1, is: ", cnM1)
	}
	validateChanges(t, changesM1, []string{}, []string{})

	// MOVE TO CN=1
	summaries.AddChanges(
		[]dtos.SplitDTO{
			{Name: "s1", TrafficTypeName: "tt1"},
			{Name: "s2", TrafficTypeName: "tt1"},
			{Name: "s3", TrafficTypeName: "tt1"},
		},
		nil,
		1,
	)

	changesM1, cnM1, err = summaries.FetchSince(-1)
	if err != nil {
		t.Error(err)
	}
	if cnM1 != 1 {
		t.Error("new CN should be 1")
	}
	validateChanges(t, changesM1, []string{"s1", "s2", "s3"}, []string{})

	changes1, cn1, err := summaries.FetchSince(1)
	if err != nil {
		t.Error(err)
	}
	if cn1 != 1 {
		t.Error("cn should be 1, is: ", cn1)
	}
	validateChanges(t, changes1, []string{}, []string{})

	// MOVE TO CN=2
	summaries.AddChanges([]dtos.SplitDTO{{Name: "s2", TrafficTypeName: "tt2"}}, nil, 2)
	changesM1, cnM1, err = summaries.FetchSince(-1)
	if err != nil {
		t.Error(err)
	}
	if cnM1 != 2 {
		t.Error("cn should be 2, is: ", cn1)
	}
	validateChanges(t, changesM1, []string{"s1", "s2", "s3"}, []string{})

	changes1, cn1, err = summaries.FetchSince(1)
	if err != nil {
		t.Error(err)
	}
	if cn1 != 2 {
		t.Error("cn should be 2, is: ", cn1)
	}
	validateChanges(t, changes1, []string{"s2"}, []string{})

	changes2, cn2, err := summaries.FetchSince(2)
	if err != nil {
		t.Error(err)
	}
	if cn2 != 2 {
		t.Error("cn should be 2, is: ", cn1)
	}
	validateChanges(t, changes2, []string{}, []string{})

	// MOVE TO CN=3
	summaries.AddChanges([]dtos.SplitDTO{{Name: "s3", TrafficTypeName: "tt3"}}, nil, 3)
	changesM1, cnM1, err = summaries.FetchSince(-1)
	if err != nil {
		t.Error(err)
	}
	if cnM1 != 3 {
		t.Error("cn should be 3, is: ", cnM1)
	}
	validateChanges(t, changesM1, []string{"s1", "s2", "s3"}, []string{})

	changes1, cn1, err = summaries.FetchSince(1)
	if err != nil {
		t.Error(err)
	}
	if cn1 != 3 {
		t.Error("cn should be 3, is: ", cn1)
	}
	validateChanges(t, changes1, []string{"s2", "s3"}, []string{})

	changes2, cn2, err = summaries.FetchSince(2)
	if err != nil {
		t.Error(err)
	}
	if cn2 != 3 {
		t.Error("cn should be 3, is: ", cn2)
	}
	validateChanges(t, changes2, []string{"s3"}, []string{})

	changes3, cn3, err := summaries.FetchSince(3)
	if err != nil {
		t.Error(err)
	}
	if cn3 != 3 {
		t.Error("cn should be 3, is: ", cn3)
	}
	validateChanges(t, changes3, []string{}, []string{})

	// MOVE TO CN=4
	summaries.AddChanges(
		[]dtos.SplitDTO{{Name: "s4", TrafficTypeName: "tt3"}},
		[]dtos.SplitDTO{{Name: "s1", TrafficTypeName: "tt1"}},
		4)
	changesM1, cnM1, err = summaries.FetchSince(-1)
	if err != nil {
		t.Error(err)
	}
	if cnM1 != 4 {
		t.Error("cn should be 4, is: ", cnM1)
	}
	validateChanges(t, changesM1, []string{"s2", "s3", "s4"}, []string{})

	changes1, cn1, err = summaries.FetchSince(1)
	if err != nil {
		t.Error(err)
	}
	if cn1 != 4 {
		t.Error("cn should be 4, is: ", cn1)
	}
	validateChanges(t, changes1, []string{"s2", "s3", "s4"}, []string{"s1"})

	changes2, cn2, err = summaries.FetchSince(2)
	if err != nil {
		t.Error(err)
	}
	if cn2 != 4 {
		t.Error("cn should be 4, is: ", cn2)
	}
	validateChanges(t, changes2, []string{"s3", "s4"}, []string{"s1"})

	changes3, cn3, err = summaries.FetchSince(3)
	if err != nil {
		t.Error(err)
	}
	if cn3 != 4 {
		t.Error("cn should be 4, is: ", cn3)
	}
	validateChanges(t, changes3, []string{"s4"}, []string{"s1"})

	changes4, cn4, err := summaries.FetchSince(4)
	if err != nil {
		t.Error(err)
	}
	if cn4 != 4 {
		t.Error("cn should be 4, is: ", cn4)
	}
	validateChanges(t, changes4, []string{}, []string{})

	// TODO: Continue test plan up to 6!
}

/*  TEST PLAN!
-1: null
1:
    ops:
        - add(s1)
        - add(s2)
        - add(s3)
    returns:
        -1: [+s1, +s2, +s3]
        1:  []
2:
    ops:
        - update(s2)
    returns:
        -1: [+s1, +s2, +s3]
        1:  [+s2]
        2:  []
3:
    ops:
        - kill(s3)
    returns:
        -1: [+s1, +s2, +s3]
        1:  [+s2, +s3]
        2:  [+s3]
        3:  []
4:
    ops:
        - add(s4)
        - del(s1)
    returns:
        -1: [+s2, +s3, +s4]
        1:  [+s2, +s3, -s1, +s4]
        2:  [+s3, -s1, +s4]
        3:  [-s1, +s4]
        4:  []
5:
    ops:
        del(s4)
    returns:
        -1: [+s2, +s3]
        1:  [+s2, +s3, -s1]
        2:  [+s3, -s1]
        3:  [-s1]
        4:  [-s4]
        5:  []
6:
    ops:
        restore(s1)
    returns:
        -1: [+s2, +s3, +s1]
        1:  [+s2, +s3, +s1]
        2:  [+s3, +s1]
        3:  [+s1]
        4:  [-s4, +s1]
        5:  [+s1]
        6:  []
*/
