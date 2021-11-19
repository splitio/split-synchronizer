package persistent

import (
	"encoding/binary"
)

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
