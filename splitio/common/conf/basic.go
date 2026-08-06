package conf

import (
	"flag"
)

// CliFlags defines the basic set of flags that are independent on the binary being executed & config required
type CliFlags struct {
	ConfigFile             *string
	WriteDefaultConfigFile *string
	VersionInfo            *bool
	RawConfig              ArgMap
}

// ParseCliArgs accepts a config options struct, parses it's definition (types + metadata) and builds the appropriate
// flag definitions. It then parses the flags, and returns the structure filled with argument values
func ParseCliArgs(definition interface{}) *CliFlags {
	flags := &CliFlags{
		ConfigFile:             flag.String("config", "", "a configuration file"),
		WriteDefaultConfigFile: flag.String("write-default-config", "", "write a default configuration file"),
		VersionInfo:            flag.Bool("version", false, "Print the version"),
		RawConfig:              MakeCliArgMapFor(definition),
	}

	flag.Parse()
	return flags
}
