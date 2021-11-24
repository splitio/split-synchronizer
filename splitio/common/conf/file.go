package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	validator "github.com/splitio/go-toolkit/v5/json-struct-validator"
)

// ErrNoFile is the error to return when an empty config file si passed
var ErrNoFile = errors.New("no config file provided")

// PopulateConfigFromFile parses a json config file and populates the config struct passed as an argument
func PopulateConfigFromFile(path string, target interface{}) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("error looking for config file (%s): %w", path, err)
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file (%s): %w", path, err)
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return fmt.Errorf("error parsing JSON config file (%s): %w", path, err)
	}

	// This function does a couple of things (to keep the caller clean):
	// - read the config file
	// - populate the struct with the appropriate values
	// - check that there are no extra fields in the json file
	// On top of that, the function needs to be generic, to work with different configs (sync || proxy).
	// The function needs a pointer to a struct to update it (modify it's contents) and it needs an actual struct
	// to inspect the fields. The `target` interface (which is already based on a pinter) parameter contains a pointer as well
	// to the struct being populated/validated. In order to validate, we need an `interface{}` object pointing to the same struct,
	// but without the extra indirection
	targetForValidation := reflect.Indirect(reflect.ValueOf(target)).Interface()
	err = validator.ValidateConfiguration(targetForValidation, data)
	if err != nil {
		return fmt.Errorf("error validanting provided JSON file (%s): %w", path, err)
	}

	return nil
}

// WriteDefaultConfigFile writes the default config defition to a JSON file
func WriteDefaultConfigFile(name string, definition interface{}) error {
	if name == "" {
		return ErrNoFile
	}

	if err := PopulateDefaults(definition); err != nil {
		return fmt.Errorf("error populating defaults: %w", err)
	}

	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return fmt.Errorf("error parsing definition: %w", err)
	}

	if err := ioutil.WriteFile(name, data, 0644); err != nil {
		return fmt.Errorf("error writing defaults to file: %w", err)
	}

	return nil
}
