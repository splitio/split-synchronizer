package log

import (
	"testing"

	"github.com/splitio/go-toolkit/v5/testhelpers"
)

func TestHistoricBuffer(t *testing.T) {
	hb := newHistoricBuffer(true, 3)
	hb.record("a")
	hb.record("b")
	hb.record("c")
	testhelpers.AssertStringSliceEquals(t, hb.messages(), []string{"a", "b", "c"}, "slices should match")

	hb.record("d")
	testhelpers.AssertStringSliceEquals(t, hb.messages(), []string{"b", "c", "d"}, "slices should match")

	if hb.count != 3 || hb.start != 1 || len(hb.buffer) != 3 {
		t.Error("incorrect values in vars ", hb.count, hb.start, len(hb.buffer))
	}

	hb.record("e")
	testhelpers.AssertStringSliceEquals(t, hb.messages(), []string{"c", "d", "e"}, "slices should match")

	if hb.count != 3 || hb.start != 2 || len(hb.buffer) != 3 {
		t.Error("incorrect values in vars ", hb.count, hb.start, len(hb.buffer))
	}

	hb.record("f")
	testhelpers.AssertStringSliceEquals(t, hb.messages(), []string{"d", "e", "f"}, "slices should match")

	if hb.count != 3 || hb.start != 0 || len(hb.buffer) != 3 {
		t.Error("incorrect values in vars ", hb.count, hb.start, len(hb.buffer))
	}

	hb.record("g")
	testhelpers.AssertStringSliceEquals(t, hb.messages(), []string{"e", "f", "g"}, "slices should match")

	if hb.count != 3 || hb.start != 1 || len(hb.buffer) != 3 {
		t.Error("incorrect values in vars ", hb.count, hb.start, len(hb.buffer))
	}

}
