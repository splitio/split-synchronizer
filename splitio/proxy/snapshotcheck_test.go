package proxy

import (
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
)

func TestCheckSnapshot(t *testing.T) {
	payload := []byte("bolt-bytes")
	valid := snapshot.Metadata{
		Checksum:        snapshot.PayloadChecksum(payload),
		FlagSpecVersion: "1.3",
	}

	if err := checkSnapshot(valid, payload, "1.2", true); err != nil {
		t.Fatalf("offline snapshot with a supported spec should load: %v", err)
	}
	if err := checkSnapshot(valid, payload, "1.3", false); err != nil {
		t.Fatalf("connected snapshot with a matching spec should load: %v", err)
	}
	if err := checkSnapshot(valid, payload, "1.2", false); err == nil {
		t.Fatal("connected mode should reject a spec that differs from configuration")
	}

	old := snapshot.Metadata{}
	if err := checkSnapshot(old, payload, "1.3", false); err != nil {
		t.Fatalf("connected mode should load a snapshot without the new fields: %v", err)
	}
	if err := checkSnapshot(old, payload, "1.3", true); err == nil {
		t.Fatal("offline mode should reject a snapshot without a checksum")
	}

	badSum := valid
	badSum.Checksum = "00"
	if err := checkSnapshot(badSum, payload, "1.3", true); err == nil {
		t.Fatal("a checksum mismatch should fail")
	}

	unsupported := valid
	unsupported.FlagSpecVersion = "9.9"
	if err := checkSnapshot(unsupported, payload, "1.3", true); err == nil {
		t.Fatal("an unsupported spec should fail")
	}
}
