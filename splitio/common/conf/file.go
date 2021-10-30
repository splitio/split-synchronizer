package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

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

	err = validator.ValidateConfiguration(target, data)
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
