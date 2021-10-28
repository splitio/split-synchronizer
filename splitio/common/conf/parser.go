package conf

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	tagNested      = "s-nested"
	tagDefault     = "s-def"
	tagCliArgName  = "s-cli"
	tagDescription = "s-desc"

	typeString      = "string"
	typeStringSlice = "[]string"
	typeInt64       = "int64"
	typeBool        = "bool"
)

// MakeCliArgMapFor returns a list of cli parameter struct
func MakeCliArgMapFor(source interface{}) ArgMap {
	val := reflect.ValueOf(source).Elem()
	return cliParametersRecursive(val)
}

// PopulateDefaults iterates the passed structure and populates the fields with the value defined in the `s-def` tag
func PopulateDefaults(target interface{}) error {
	var configDataReflection = reflect.ValueOf(target).Elem()
	loadDefaultValuesRecursive(configDataReflection)
	return nil
}

// PopulateFromArguments examines target fields by reflection and populates them with the contents of argMap
func PopulateFromArguments(target interface{}, argMap ArgMap) {
	populateFromArgsRecursive(reflect.ValueOf(target).Elem(), argMap)
}

// func loadFile(path string) error {
// 	if _, err := os.Stat(path); !os.IsNotExist(err) {
// 		dat, err := ioutil.ReadFile(path)
// 		if err != nil {
// 			fmt.Println(err.Error())
// 		}
// 		err = json.Unmarshal(dat, &Data)
// 		if err != nil {
// 			fmt.Println(err.Error())
// 		}
//
// 		var Config ConfigData
// 		err = validator.ValidateConfiguration(Config, dat)
// 		if err != nil {
// 			fmt.Println(err.Error())
// 			return err
// 		}
// 	}
// 	return nil
// }

// LoadFromFile configuration values from file
// func LoadFromFile(path string) error {
// 	return loadFile(path)
// }

// ArgMap is a type alias used to hold values parsed from CLI arguments, used to populate a config structure
type ArgMap map[string]interface{}

func (m ArgMap) getBool(name string) (bool, bool) {
	v, isPresent := m[name]
	if !isPresent {
		return false, false
	}

	asBoolPointer, ok := v.(*bool)
	if !ok || asBoolPointer == nil {
		return false, false
	}

	return *asBoolPointer, true
}

func (m ArgMap) getString(name string) (string, bool) {
	v, isPresent := m[name]
	if !isPresent {
		return "", false
	}

	asStrPointer, ok := v.(*string)
	if !ok || asStrPointer == nil {
		return "", false
	}

	return *asStrPointer, true
}

func (m ArgMap) getInt64(name string) (int64, bool) {
	v, isPresent := m[name]
	if !isPresent {
		return 0, false
	}

	asIntPointer, ok := v.(*int64)
	if !ok || asIntPointer == nil {
		return 0, false
	}

	return *asIntPointer, true
}

func (m ArgMap) getStringSlice(name string) ([]string, bool) {
	v, isPresent := m[name]
	if !isPresent {
		return nil, false
	}

	asStrPointer, ok := v.(*string)
	if !ok || asStrPointer == nil {
		return nil, false
	}

	return strings.Split(*asStrPointer, ","), true
}

func loadDefaultValuesRecursive(val reflect.Value) {
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// Handle nested structs
		if len(tag.Get(tagNested)) > 0 {
			loadDefaultValuesRecursive(valueField)
		}

		// load node
		def := tag.Get(tagDefault)
		if len(def) <= 0 {
			continue
		}
		attributeType := fmt.Sprintf("%s", typeField.Type)
		switch attributeType {
		case typeString:
			val.Field(i).SetString(def)
			break
		case typeStringSlice:
			auxSlice := defaultStringSliceFromString(def)
			rval := reflect.MakeSlice(typeField.Type, len(auxSlice), cap(auxSlice))
			for idx, v := range auxSlice {
				rval.Index(idx).SetString(v)
			}
			val.Field(i).Set(rval)
			break
		case typeInt64:
			val.Field(i).SetInt(defaultInt64FromString(def))
			break
		case typeBool:
			val.Field(i).SetBool(defaultBoolFromString(def))
			break
		}
	}
}

func populateFromArgsRecursive(val reflect.Value, cliParametersMap ArgMap) {
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// load child
		if len(tag.Get(tagNested)) > 0 {
			populateFromArgsRecursive(valueField, cliParametersMap)
		}

		// If the current property does not have a CLI-mapping, ignore
		cliArgName := tag.Get(tagCliArgName)
		if len(cliArgName) <= 0 {
			continue
		}

		attributeType := fmt.Sprintf("%s", typeField.Type)
		defaultVal := tag.Get(tagDefault)
		switch attributeType {
		case typeString:
			v, ok := cliParametersMap.getString(cliArgName)
			if ok && v != defaultVal {
				val.Field(i).SetString(v)
			}
			break
		case typeStringSlice:
			v, ok := cliParametersMap.getStringSlice(cliArgName)
			if ok && !strSliceEquals(v, strings.Split(defaultVal, ",")) {
				rval := reflect.MakeSlice(typeField.Type, len(v), cap(v))
				for idx, item := range v {
					rval.Index(idx).SetString(item)
				}
				val.Field(i).Set(rval)
			}
			break
		case typeInt64:
			v, ok := cliParametersMap.getInt64(cliArgName)
			defaultValInt64, _ := strconv.ParseInt(defaultVal, 10, 64)
			if ok && v != defaultValInt64 {
				val.Field(i).SetInt(int64(v))
			}
			break
		case typeBool:
			v, ok := cliParametersMap.getBool(cliArgName)
			defaultValBool, _ := strconv.ParseBool(defaultVal)
			if ok && v != defaultValBool {
				val.Field(i).SetBool(v)
			}
			break
		}
	}
}

// LoadFromArgs loads configuration values from cli
// func LoadFromArgs(cliParametersMap map[string]interface{}) {
// 	// getting reflection pointer to configuration data struct
// 	var configDataReflection = reflect.ValueOf(&Data).Elem()
// 	loadFromArgsRecursiveChildren(configDataReflection, cliParametersMap)
// }

func cliParametersRecursive(val reflect.Value) ArgMap {
	var toReturn = make(ArgMap)

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		// If it's a nested structure, process the child members and then merge the results
		if len(tag.Get(tagNested)) > 0 {
			children := cliParametersRecursive(valueField)
			for k, v := range children {
				toReturn[k] = v
			}
		}

		cliArgName := tag.Get(tagCliArgName)
		if len(cliArgName) <= 0 {
			continue
		}

		def := tag.Get(tagDefault)
		desc := tag.Get(tagDescription)
		switch typeField.Type.String() {
		case typeString, typeStringSlice: // flags for string & []string are set as strings
			toReturn[cliArgName] = flag.String(cliArgName, def, desc)
		case typeInt64:
			toReturn[cliArgName] = flag.Int64(cliArgName, defaultInt64FromString(def), desc)
		case typeBool:
			toReturn[cliArgName] = flag.Bool(cliArgName, defaultBoolFromString(def), desc)
		}
	}
	return toReturn
}

func strSliceEquals(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	for idx := range s1 { // s1 && s2 have equal length, this is safe
		if s1[idx] != s2[idx] {
			return false
		}
	}
	return true
}

func defaultBoolFromString(str string) bool {
	res, _ := strconv.ParseBool(str)
	return res
}

func defaultInt64FromString(str string) int64 {
	res, _ := strconv.ParseInt(str, 10, 64)
	return res
}

func defaultStringSliceFromString(str string) []string {
	return strings.Split(str, ",")
}
