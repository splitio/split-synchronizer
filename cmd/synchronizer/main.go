package main

import (
	"errors"
	"fmt"
	"github.com/splitio/go-split-commons/v5/flagsets"
	"os"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio"
	"github.com/splitio/split-synchronizer/v5/splitio/common"
	cconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/log"
	"github.com/splitio/split-synchronizer/v5/splitio/producer"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
)

const (
	exitCodeSuccess     = 0
	exitCodeConfigError = 1
)

func parseCliArgs() *cconf.CliFlags {
	return cconf.ParseCliArgs(&conf.Main{})
}

type flagSetValidationError struct {
	wrapped []error
}

func (f flagSetValidationError) Error() string {
	var errors []string
	for _, err := range f.wrapped {
		errors = append(errors, err.Error())
	}
	return strings.Join(errors, ".|| ")
}

func setupConfig(cliArgs *cconf.CliFlags) (*conf.Main, error) {
	syncConf := conf.Main{}
	cconf.PopulateDefaults(&syncConf)

	if path := *cliArgs.ConfigFile; path != "" {
		err := cconf.PopulateConfigFromFile(path, &syncConf)
		if err != nil {
			return nil, fmt.Errorf("error parsing config file: %w", err)
		}
	}

	cconf.PopulateFromArguments(&syncConf, cliArgs.RawConfig)

	var err error
	sanitizedFlagSets, fsErr := flagsets.SanitizeMany(syncConf.FlagSetsFilter)
	if fsErr != nil {
		err = flagSetValidationError{wrapped: fsErr}
	}
	if sanitizedFlagSets != nil {
		syncConf.FlagSetsFilter = sanitizedFlagSets
	}
	return &syncConf, err
}

func main() {
	fmt.Println(splitio.ASCILogo)
	fmt.Printf("\nSplit Synchronizer - Version: %s (%s) \n", splitio.Version, splitio.CommitVersion)

	cliArgs := parseCliArgs()
	if *cliArgs.VersionInfo {
		os.Exit(exitCodeSuccess)
	}

	if fn := *cliArgs.WriteDefaultConfigFile; fn != "" {
		if err := cconf.WriteDefaultConfigFile(fn, &conf.Main{}); err != nil {
			fmt.Printf("error writing config file with default values: %s", err.Error())
			os.Exit(exitCodeConfigError)
		}
		fmt.Println("Configuration file written successfully to: ", fn)
		os.Exit(exitCodeSuccess)
	}

	cfg, err := setupConfig(cliArgs)
	if err != nil {
		var fsErr flagSetValidationError
		if errors.As(err, &fsErr) {
			fmt.Println("error processing flagset: ", err.Error())
		} else {
			fmt.Println("error processing config: ", err)
			os.Exit(exitCodeConfigError)
		}
	}

	logger := log.BuildFromConfig(&cfg.Logging, "Split-Sync", &cfg.Integrations.Slack)
	err = producer.Start(logger, cfg)

	if err == nil {
		return
	}

	var initError *common.InitializationError
	if errors.As(err, &initError) {
		logger.Error("Failed to initialize the split sync: ", initError)
		os.Exit(initError.ExitCode())
	}

	os.Exit(common.ExitUndefined)
}
