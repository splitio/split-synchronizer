package conf

import (
	"testing"

	"github.com/splitio/go-split-commons/v9/dtos"

	"github.com/stretchr/testify/assert"
)

func TestFlagSetValidationError(t *testing.T) {

	sanitized, err := ValidateFlagsets([]string{"Flagset1", " flagset2 ", "123#@flagset"})
	assert.NotNil(t, err)
	assert.Equal(t, []string{"flagset1", "flagset2"}, sanitized)

	asFVE := err.(FlagSetValidationError)
	assert.Equal(t, 3, len(asFVE.wrapped))
	assert.ElementsMatch(t, []error{
		dtos.FlagSetValidatonError{Message: "Flag Set name Flagset1 should be all lowercase - converting string to lowercase"},
		dtos.FlagSetValidatonError{Message: "Flag Set name  flagset2  has extra whitespace, trimming"},
		dtos.FlagSetValidatonError{Message: "you passed 123#@flagset, Flag Set must adhere to the regular expressions ^[a-z0-9][_a-z0-9]{0,49}$. This means a Flag Set must " +
			"start with a letter or number, be in lowercase, alphanumeric and have a max length of 50 characters. 123#@flagset was discarded."},
	}, asFVE.wrapped)
}
