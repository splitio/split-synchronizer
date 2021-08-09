package conf

import (
	"fmt"
	"math"
	"os"
	"strings"

	cfg "github.com/splitio/go-split-commons/v3/conf"
)

const (
	defaultImpressionSyncOptimized = 300
	defaultImpressionSync          = 60
	minImpressionSyncDebug         = 1
)

func checkImpressionsPostRate() error {
	if Data.ImpressionsPostRate == 0 {
		Data.ImpressionsPostRate = defaultImpressionSyncOptimized
	} else {
		if Data.ImpressionsPostRate < defaultImpressionSync {
			return fmt.Errorf("ImpressionsPostRate must be >= %d. Actual is: %d", defaultImpressionSync, Data.ImpressionsPostRate)
		}
		Data.ImpressionsPostRate = int(math.Max(float64(defaultImpressionSync), float64(Data.ImpressionsPostRate)))
	}
	return nil
}

// ValidConfigs checks configs
func ValidConfigs() error {
	var impressionsPostRateError error
	Data.ImpressionsMode = strings.ToLower(Data.ImpressionsMode)
	switch Data.ImpressionsMode {
	case cfg.ImpressionsModeOptimized:
		impressionsPostRateError = checkImpressionsPostRate()
	case cfg.ImpressionsModeDebug:
		if Data.ImpressionsPostRate == 0 {
			Data.ImpressionsPostRate = defaultImpressionSync
		} else {
			if Data.ImpressionsPostRate < minImpressionSyncDebug {
				return fmt.Errorf("ImpressionsPostRate must be >= %d. Actual is: %d", minImpressionSyncDebug, Data.ImpressionsPostRate)
			}
		}
	default:
		fmt.Println(`You passed an invalid impressionsMode, impressionsMode should be one of the following values: 'debug' or 'optimized'. Defaulting to 'optimized' mode.`)
		Data.ImpressionsMode = cfg.ImpressionsModeOptimized
		impressionsPostRateError = checkImpressionsPostRate()
	}

	if impressionsPostRateError != nil {
		return impressionsPostRateError
	}

	// Snapshot validation
	if Data.Proxy.Snapshot != "" {
		if !snapshotExists(Data.Proxy.Snapshot) {
			return fmt.Errorf("snapshot file does not exists at %s", Data.Proxy.Snapshot)
		}
	} else { //TODO (sarrubia) remove Data.Proxy.PersistMemoryPath on next versions, this is replaced by Snapshot
		Data.Proxy.Snapshot = Data.Proxy.PersistMemoryPath
	}

	return nil
}

// snapshotExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func snapshotExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}