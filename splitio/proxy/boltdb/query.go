package boltdb

// AND operation for index query results
func AND(s1 []uint64, s2 []uint64) []uint64 {
	return intersection(s1, s2)
}

// OR operation for index query results
func OR(s1 []uint64, s2 []uint64) []uint64 {
	return union(s1, s2)
}

// NOTIN operation for index query results
// Returns elements from s1 that are not in s2
func NOTIN(s1 []uint64, s2 []uint64) []uint64 {
	return complement(s1, s2)
}
