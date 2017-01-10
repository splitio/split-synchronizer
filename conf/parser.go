// Package conf implements functions to read configuration data
package conf

import (
	"encoding/json"
	"io/ioutil"

	"github.com/splitio/go-agent/errors"
)

// Data contains all configuration values
var Data ConfigData

func loadFile(path string) {
	dat, err := ioutil.ReadFile(path)
	errors.CheckError(err)

	Data = ConfigData{}
	err = json.Unmarshal(dat, &Data)
	errors.CheckError(err)
}

// Load configuration file into struct
func Load(path string) {
	loadFile(path)
}
