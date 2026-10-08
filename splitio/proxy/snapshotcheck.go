package proxy

import (
	"fmt"

	"github.com/splitio/go-split-commons/v10/service/api/specs"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
)

// checkSnapshot validates checksum and flag spec metadata.
// Offline snapshots must have both fields. Connected snapshots check them only when present.
func checkSnapshot(meta snapshot.Metadata, payload []byte, configuredSpec string, offline bool) error {
	if meta.Checksum == "" {
		if offline {
			return fmt.Errorf("offline mode requires a snapshot checksum")
		}
	} else if meta.Checksum != snapshot.PayloadChecksum(payload) {
		return fmt.Errorf("snapshot checksum does not match payload")
	}

	if meta.FlagSpecVersion == "" {
		if offline {
			return fmt.Errorf("offline mode requires a snapshot flag spec version")
		}
		return nil
	}

	if specs.Match(meta.FlagSpecVersion) == nil {
		return fmt.Errorf("snapshot flag spec version %q is not supported", meta.FlagSpecVersion)
	}
	if !offline && meta.FlagSpecVersion != configuredSpec {
		return fmt.Errorf("snapshot flag spec version %q does not match configured spec %q", meta.FlagSpecVersion, configuredSpec)
	}
	return nil
}
