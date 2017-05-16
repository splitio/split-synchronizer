package boltdb

import "encoding/binary"

// itob returns an 8-byte big endian representation of v.
func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// btoi returns an uint64 from its 8-byte big endian representation.
func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func complement(s1 []uint64, s2 []uint64) []uint64 {
	diffInt := []uint64{}
	m := map[uint64]int{}

	for _, s1Val := range s1 {
		m[s1Val] = 1
	}
	for _, s2Val := range s2 {
		m[s2Val] = m[s2Val] + 2
	}

	for mKey, mVal := range m {
		if mVal == 1 {
			diffInt = append(diffInt, mKey)
		}
	}

	return diffInt
}

func difference(s1 []uint64, s2 []uint64) []uint64 {
	diffInt := []uint64{}
	m := map[uint64]int{}

	for _, s1Val := range s1 {
		m[s1Val] = 1
	}
	for _, s2Val := range s2 {
		m[s2Val] = m[s2Val] + 1
	}

	for mKey, mVal := range m {
		if mVal == 1 {
			diffInt = append(diffInt, mKey)
		}
	}

	return diffInt
}

func intersection(s1 []uint64, s2 []uint64) []uint64 {
	diffInt := []uint64{}
	m := map[uint64]int{}

	for _, s1Val := range s1 {
		m[s1Val] = 1
	}
	for _, s2Val := range s2 {
		m[s2Val] = m[s2Val] + 1
	}

	for mKey, mVal := range m {
		if mVal == 2 {
			diffInt = append(diffInt, mKey)
		}
	}

	return diffInt
}

func union(s1 []uint64, s2 []uint64) []uint64 {
	diffInt := []uint64{}
	m := make(map[uint64]struct{})

	for _, s1Val := range s1 {
		m[s1Val] = struct{}{}
	}
	for _, s2Val := range s2 {
		m[s2Val] = struct{}{}
	}

	for mKey := range m {
		diffInt = append(diffInt, mKey)
	}

	return diffInt
}
