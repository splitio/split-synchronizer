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

	if decodedSnapshot.Meta().Checksum != "" || decodedSnapshot.Meta().FlagSpecVersion != "" {
		t.Error("new metadata fields should be empty when they were not set")
	}

	withChecksum := Metadata{
		Storage:         storage4Test,
		Version:         version4Test,
		Checksum:        PayloadChecksum(data4Test),
		FlagSpecVersion: "1.3",
	}
	encodedNew, err := New(withChecksum, data4Test)
	if err != nil {
		t.Error(err)
	}
	raw, err := encodedNew.Encode()
	if err != nil {
		t.Error(err)
	}
	decodedNew, err := Decode(raw)
	if err != nil {
		t.Error(err)
	}
	if decodedNew.Meta().Checksum != withChecksum.Checksum || decodedNew.Meta().FlagSpecVersion != "1.3" {
		t.Error("checksum and flag spec version were not preserved")
	}
}
