package conf

import (
	"fmt"
	"math"
	"strings"

	cfg "github.com/splitio/go-split-commons/conf"
)

const (
	defaultImpressionSyncOptimized = 300
	defaultImpressionSync          = 60
	minImpressionSyncDebug         = 1
)

// ValidConfigs checks configs
func ValidConfigs() error {
	Data.ImpressionsMode = strings.ToLower(Data.ImpressionsMode)
	if Data.ImpressionsMode != cfg.ImpressionsModeDebug && Data.ImpressionsMode != cfg.ImpressionsModeOptimized {
		fmt.Println(`You passed an invalid impressionsMode, impressionsMode should be one of the following values: 'debug' or 'optimized'. Defaulting to 'optimized' mode.`)
		Data.ImpressionsMode = cfg.ImpressionsModeOptimized
	}

	switch Data.ImpressionsMode {
	case cfg.ImpressionsModeOptimized:
		if Data.ImpressionsPostRate == 0 {
			Data.ImpressionsPostRate = defaultImpressionSyncOptimized
		} else {
			if Data.ImpressionsPostRate < defaultImpressionSync {
				return fmt.Errorf("ImpressionsPostRate must be >= %d. Actual is: %d", defaultImpressionSync, Data.ImpressionsPostRate)
			}
			Data.ImpressionsPostRate = int(math.Max(float64(defaultImpressionSync), float64(Data.ImpressionsPostRate)))
		}
	case cfg.ImpressionsModeDebug:
		fallthrough
	default:
		if Data.ImpressionsPostRate == 0 {
			Data.ImpressionsPostRate = defaultImpressionSync
		} else {
			if Data.ImpressionsPostRate < minImpressionSyncDebug {
				return fmt.Errorf("ImpressionsPostRate must be >= %d. Actual is: %d", minImpressionSyncDebug, Data.ImpressionsPostRate)
			}
			Data.ImpressionsPostRate = int(math.Max(float64(defaultImpressionSync), float64(Data.ImpressionsPostRate)))
		}
	}
	return nil
}
