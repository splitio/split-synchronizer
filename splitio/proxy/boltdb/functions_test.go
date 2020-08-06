package boltdb

import (
	"testing"
)

func TestKeyConvertion(t *testing.T) {
	var key uint64 = 163456
	keyBytes := KeyInt(key)
	decodedKey := btoi(keyBytes)

	if key != decodedKey {
		t.Error("Error encoding/decoding key")
	}
}

func isInSlice(s []uint64, v uint64) bool {
	for _, val := range s {
		if v == val {
			return true
		}
	}
	return false
}

func TestComplement(t *testing.T) {
	//complement(s1 []uint64, s2 []uint64) []uint64
	var s1 = []uint64{1, 2, 3, 4, 5, 6}
	var s2 = []uint64{1, 2, 3, 0, 0, 6}

	compResult := complement(s1, s2)
	if !isInSlice(compResult, 4) || !isInSlice(compResult, 5) {
		t.Error("Complement function error")
	}
}

func TestDifference(t *testing.T) {
	var s1 = []uint64{1, 2, 3, 4, 5, 6}
	var s2 = []uint64{1, 2, 3, 4, 0, 6}

	result := difference(s1, s2)
	if !isInSlice(result, 5) || !isInSlice(result, 0) {
		t.Error("Difference function error")
	}
}

func TestIntersection(t *testing.T) {
	var s1 = []uint64{1, 2, 3, 4, 5, 6}
	var s2 = []uint64{0, 0, 0, 4, 5, 0}

	result := intersection(s1, s2)
	if !isInSlice(result, 5) || !isInSlice(result, 4) {
		t.Error("Intersection function error")
	}
}

func TestUnion(t *testing.T) {
	var s1 = []uint64{1, 2}
	var s2 = []uint64{1, 3, 4}

	result := union(s1, s2)
	if !isInSlice(result, 1) || !isInSlice(result, 2) || !isInSlice(result, 3) || !isInSlice(result, 4) {
		t.Error("Union function error")
	}
}
