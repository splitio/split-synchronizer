package conf

import (
	"testing"

	"github.com/splitio/go-split-commons/v4/conf"
)

func TestValidator(t *testing.T) {
	Initialize()
	Data.ImpressionsMode = "some"

	err := ValidConfigs()
	if err != nil {
		t.Error("It should not return err")
	}
	if Data.ImpressionsMode != conf.ImpressionsModeOptimized {
		t.Error("It should be optimized")
	}
}

func TestValidatorWrongRatesInOptimized(t *testing.T) {
	Initialize()
	Data.ImpressionsPostRate = 10

	err := ValidConfigs()
	if err == nil || err.Error() != "ImpressionsPostRate must be >= 60. Actual is: 10" {
		t.Error("It should return err. Got:", err)
	}
	if Data.ImpressionsMode != conf.ImpressionsModeOptimized {
		t.Error("It should be optimized")
	}
}
