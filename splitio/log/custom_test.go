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
	testhelpers.AssertStringSliceEquals(t, []string{"a", "b", "c"}, hb.messages(), "slices should match")

	hb.record("d")
	testhelpers.AssertStringSliceEquals(t, []string{"b", "c", "d"}, hb.messages(), "slices should match")

	if hb.count != 3 || hb.start != 1 || len(hb.buffer) != 3 {
		t.Error("incorrect values in vars ", hb.count, hb.start, hb.buffer)
	}
}
