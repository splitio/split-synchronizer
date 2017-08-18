package boltdb

import "testing"

func TestAND(t *testing.T) {
	var s1 = []uint64{1, 2, 3, 4, 5, 6}
	var s2 = []uint64{0, 0, 0, 4, 5, 0}

	result := AND(s1, s2)
	if !isInSlice(result, 5) || !isInSlice(result, 4) {
		t.Error("AND function error")
	}
}

func TestOR(t *testing.T) {
	var s1 = []uint64{1, 2}
	var s2 = []uint64{1, 3, 4}

	result := OR(s1, s2)
	if !isInSlice(result, 1) || !isInSlice(result, 2) || !isInSlice(result, 3) || !isInSlice(result, 4) {
		t.Error("OR function error")
	}
}

func TestNOTIN(t *testing.T) {
	var s1 = []uint64{1, 2, 3, 4, 5, 6}
	var s2 = []uint64{1, 2, 3, 0, 0, 6}

	result := NOTIN(s1, s2)
	if !isInSlice(result, 4) || !isInSlice(result, 5) {
		t.Error("NOT IN function error")
	}
}
