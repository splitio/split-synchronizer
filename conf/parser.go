// Package conf implements functions to read configuration data
package conf

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"

	"github.com/splitio/go-agent/errors"
)

// Data contains all configuration values
var Data ConfigData

// CommandConfigData represent a command line data structure
type CommandConfigData struct {
	Command       string
	Description   string
	Attribute     string
	AttributeType string
	DefaultValue  interface{}
}

// NewInitializedConfigData returns an initialized by default config struct
func NewInitializedConfigData() ConfigData {
	return getDefaultConfigData()
}

func loadFile(path string) {
	dat, err := ioutil.ReadFile(path)
	errors.CheckError(err)

	Data = getDefaultConfigData()
	err = json.Unmarshal(dat, &Data)
	errors.CheckError(err)
}

// LoadFromFile configuration values from file
func LoadFromFile(path string) {
	loadFile(path)
}

func loadDefaultValuesRecursiveChildren(val reflect.Value) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// load child
		if len(tag.Get("split-cli-option-group")) > 0 {
			loadDefaultValuesRecursiveChildren(valueField)
		}

		// load node
		if len(tag.Get("split-default-value")) > 0 {
			attributeType := fmt.Sprintf("%s", typeField.Type)
			defaultVal := tag.Get("split-default-value")
			switch attributeType {
			case "string":
				val.Field(i).SetString(defaultVal)
				break
			case "int":
				defaultValInt, _ := strconv.Atoi(defaultVal)
				val.Field(i).SetInt(int64(defaultValInt))
				break
			case "int64":
				defaultValInt64, _ := strconv.ParseInt(defaultVal, 10, 64)
				val.Field(i).SetInt(int64(defaultValInt64))
				break
			case "bool":
				defaultValBool, _ := strconv.ParseBool(defaultVal)
				val.Field(i).SetBool(defaultValBool)
				break
			}
		}
	}
}

func loadFromArgsRecursiveChildren(val reflect.Value, cliParametersMap map[string]interface{}) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// load child
		if len(tag.Get("split-cli-option-group")) > 0 {
			loadFromArgsRecursiveChildren(valueField, cliParametersMap)
		}

		// load node
		if len(tag.Get("split-cli-option")) > 0 {

			v, ok := cliParametersMap[tag.Get("split-cli-option")]
			if ok {
				attributeType := fmt.Sprintf("%s", typeField.Type)
				defaultVal := tag.Get("split-default-value")
				switch attributeType {
				case "string":
					var cliVal = *(v.(*string))
					if cliVal != defaultVal {
						val.Field(i).SetString(cliVal)
					}
					break
				case "int":
					var cliVal = *(v.(*int))
					defaultValInt, _ := strconv.Atoi(defaultVal)
					if cliVal != defaultValInt {
						val.Field(i).SetInt(int64(cliVal))
					}
					break
				case "int64":
					var cliVal = *(v.(*int64))
					defaultValInt64, _ := strconv.ParseInt(defaultVal, 10, 64) //param.DefaultValue.(int64)
					if cliVal != defaultValInt64 {
						val.Field(i).SetInt(int64(cliVal))
					}
					break
				case "bool":
					var cliVal = *(v.(*bool))
					defaultValBool, _ := strconv.ParseBool(defaultVal)
					if cliVal != defaultValBool {
						val.Field(i).SetBool(cliVal)
					}
					break
				}
			}
		}
	}
}

// LoadFromArgs loads configuration values from cli
func LoadFromArgs(cliParametersMap map[string]interface{}) {
	// getting reflection pointer to configuration data struct
	var configDataReflection = reflect.ValueOf(&Data).Elem()
	loadFromArgsRecursiveChildren(configDataReflection, cliParametersMap)
}

func cliParametersRecursiveChildren(val reflect.Value) map[string]CommandConfigData {
	var toReturn = make(map[string]CommandConfigData)

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// parse child
		if len(tag.Get("split-cli-option-group")) > 0 {
			commandChildren := cliParametersRecursiveChildren(valueField)
			//merging
			for k, v := range commandChildren {
				toReturn[k] = v
			}
		}

		// parse node
		if len(tag.Get("split-cli-option")) > 0 {
			toReturn[tag.Get("split-cli-option")] = CommandConfigData{
				Command:       tag.Get("split-cli-option"),
				Description:   tag.Get("split-cli-description"),
				Attribute:     typeField.Name,
				AttributeType: fmt.Sprintf("%s", typeField.Type),
				DefaultValue:  valueField.Interface()}
		}
	}

	return toReturn
}

// CliParametersToRegister returns a list of cli parameter struct
func CliParametersToRegister() map[string]CommandConfigData {
	var data = getDefaultConfigData()
	val := reflect.ValueOf(&data).Elem()
	return cliParametersRecursiveChildren(val)
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

func getDefaultConfigData() ConfigData {
	configData := ConfigData{}
	var configDataReflection = reflect.ValueOf(&configData).Elem()
	loadDefaultValuesRecursiveChildren(configDataReflection)
	return configData
}
