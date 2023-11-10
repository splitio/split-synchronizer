package main

import (
	"fmt"
	"github.com/splitio/go-split-commons/v5/flagsets"
	"golang.org/x/exp/slices"
	"strings"
	"testing"
)

func TestFlagSetValidationError(t *testing.T) {
	flagSets, err := flagsets.SanitizeMany([]string{"Flagset1", " flagset2 ", "123#@flagset"})
	if err == nil {
		t.Error("errors should not be nil")
	}
	if len(err) != 3 {
		t.Error("Unexpected Amount of errors. Should be 3. Was", len(err))
	}
	if len(flagSets) != 2 {
		t.Error("Unexpected amount of flagsets. Should be 2. Was", len(flagSets))
	}
	if !slices.Contains(flagSets, "flagset1") || !slices.Contains(flagSets, "flagset2") {
		t.Error("Missing flagsets.")
	}
	fsvError := flagSetValidationError{wrapped: err}.Error()
	if !strings.Contains(fsvError, "Flagset1") || !strings.Contains(fsvError, "flagset2") || !strings.Contains(fsvError, "123#@flagset") {
		t.Error("Missing errors on flagSetValidation.")
	}
	fmt.Printf("Flagsets: %#v", flagSets)
}
