package proxy

import (
	"fmt"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/overrides"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"

	"github.com/splitio/go-toolkit/v5/logging"
)

// setupOverrides loads and validates the overrides file against the flags in splitStorage and returns it with the
// startup stamp. Without a path it returns nil. An empty file is valid and still stamps.
func setupOverrides(
	logger logging.LoggerInterface,
	path string,
	splitStorage storage.ProxySplitStorage,
	now func() time.Time,
) (*overrides.Overrides, int64, error) {
	if path == "" {
		return nil, 0, nil
	}

	o, err := overrides.Load(path)
	if err != nil {
		return nil, 0, common.NewInitError(err, common.ExitInvalidConfiguration)
	}

	all, err := splitStorage.ChangesSince(-1, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("error reading feature flags to validate overrides: %w", err)
	}
	skipped, err := overrides.Validate(o, all.Splits)
	if err != nil {
		return nil, 0, common.NewInitError(err, common.ExitInvalidConfiguration)
	}

	stamp := overrides.Stamp(now().UnixMilli(), all.Till)
	logOverrides(logger, o, stamp, skipped)
	return o, stamp, nil
}

// logOverrides tells support which overrides are in place. Only flags named in the file are logged.
func logOverrides(logger logging.LoggerInterface, o *overrides.Overrides, stamp int64, skipped []string) {
	logger.Info(fmt.Sprintf("Overrides loaded from %s (%d entries, sha256:%s); flag change number stamped at %d",
		o.Path, len(o.Entries), o.SHA256, stamp))
	for _, flag := range o.Flags() {
		entry := o.Entries[flag]
		msg := fmt.Sprintf("Override: %s -> %q for all keys (targeting ignored)", flag, entry.Treatment)
		if entry.Config != nil {
			msg += ", config replaced"
		}
		logger.Info(msg)
	}
	for _, flag := range skipped {
		logger.Warning(fmt.Sprintf("Override for '%s' skipped: flag not present in proxy data", flag))
	}
}
