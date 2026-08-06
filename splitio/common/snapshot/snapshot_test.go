package snapshot

import "testing"

func TestSnapshot(t *testing.T) {
	data4Test := []byte("Some Snapshot Data")
	storage4Test := uint64(4321)
	version4Test := uint64(123456)
	meta4Test := Metadata{Storage: storage4Test, Version: version4Test}

	snapshot, err := New(meta4Test, data4Test)
	if err != nil {
		t.Error(err)
	}

	encoded, err := snapshot.Encode()
	if err != nil {
		t.Error(err)
	}

	decodedSnapshot, err := Decode(encoded)
	if err != nil {
		t.Error(err)
	}

	if decodedSnapshot.Meta().Storage != storage4Test {
		t.Error("Metadata Storage invalid value")
	}

	if decodedSnapshot.Meta().Version != version4Test {
		t.Error("Metadata Version invalid value")
	}

	decodedData, err := decodedSnapshot.Data()
	if err != nil {
		t.Error(err)
	}

	if string(decodedData) != string(data4Test) {
		t.Error("invalid decoded data")
	}

}
