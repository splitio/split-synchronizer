package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
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

	noSpec := valid
	noSpec.FlagSpecVersion = ""
	if err := checkSnapshot(noSpec, payload, "1.3", true); err == nil {
		t.Fatal("offline mode should reject a snapshot with a checksum but no flag spec version")
	}
	if err := checkSnapshot(noSpec, payload, "1.3", false); err != nil {
		t.Fatalf("connected mode should load a snapshot with a checksum but no spec version: %v", err)
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

func TestConnectedStartRejectsCorruptSnapshotAsInvalidConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.snapshot")
	if err := os.WriteFile(path, []byte("not a snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &pconf.Main{Apikey: "test-apikey", FlagSpecVersion: "1.3"}
	cfg.Initialization.Snapshot = path
	err := Start(&mockLogger{}, cfg)
	if err == nil {
		t.Fatal("a corrupt snapshot should stop connected startup")
	}
	initErr, ok := err.(*common.InitializationError)
	if !ok {
		t.Fatalf("expected an InitializationError so the exit is logged, got %T: %v", err, err)
	}
	if initErr.ExitCode() != common.ExitInvalidConfiguration {
		t.Errorf("exit code = %d, want %d (ExitInvalidConfiguration)", initErr.ExitCode(), common.ExitInvalidConfiguration)
	}
}
