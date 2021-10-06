package persistent

import (
	"encoding/binary"
	"testing"
)

func TestItob(t *testing.T) {
	if x := binary.BigEndian.Uint64(itob(12345)); x != 12345 {
		t.Error("should be 12345. Is: ", x)
	}
}

func TestBtoI(t *testing.T) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(12345))
	if x := btoi(b); x != 12345 {
		t.Error("should be 12345. Is: ", x)
	}
}
