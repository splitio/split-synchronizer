// Package conf implements functions to read configuration data
package conf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/splitio/go-agent/errors"
)

// Data contains all configuration values
var Data ConfigData

func loadFile(path string) {
	dat, err := ioutil.ReadFile(path)
	errors.CheckError(err)

	Data = getDefaultConfigData() //ConfigData{}
	err = json.Unmarshal(dat, &Data)
	errors.CheckError(err)
}

// Load configuration file into struct
func Load(path string) {
	loadFile(path)
}

// NewInitializedConfigData returns an initialized by default config struct
func NewInitializedConfigData() ConfigData {
	return getDefaultConfigData()
}

// CliParametersToRegister returns a list of cli parameter struct
func CliParametersToRegister() []CommandConfigData {
	var data = getDefaultConfigData()
	return data.cliParameters()
}

// WriteDefaultConfigFile writes a json file
func WriteDefaultConfigFile(path string) {
	data, err1 := getDefaultConfigData().MarshalBinary()
	if err1 != nil {
		fmt.Println(err1)
	}

	if err2 := ioutil.WriteFile(path, data, 0644); err2 != nil {
		fmt.Println(err2)
	}
}
