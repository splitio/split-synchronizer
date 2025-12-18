package conf

import (
	"strings"

	"github.com/splitio/go-split-commons/v9/flagsets"
)

type FlagSetValidationError struct {
	wrapped []error
}

func (f FlagSetValidationError) Error() string {
	var errors []string
	for _, err := range f.wrapped {
		errors = append(errors, err.Error())
	}
	return strings.Join(errors, ".|| ")
}

func ValidateFlagsets(sets []string) ([]string, error) {
	var toRet error
	sanitizedFlagSets, fsErr := flagsets.SanitizeMany(sets)
	if fsErr != nil {
		toRet = FlagSetValidationError{wrapped: fsErr}
	}
	return sanitizedFlagSets, toRet
}
