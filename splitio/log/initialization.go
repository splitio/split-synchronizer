package log

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

func meansStdout(s string) bool {
	switch strings.ToLower(s) {
	case "stdout", "/dev/stdout":
		return true
	default:
		return false
	}
}

// BuildFromConfig creates a logger from a config
func BuildFromConfig(cfg *conf.Logging, prefix string, slackCfg *conf.Slack) *HistoricLoggerWrapper {
	var err error
	var mainWriter io.Writer = os.Stdout

	if !meansStdout(cfg.Output) {
		mainWriter, err = logging.NewFileRotate(&logging.FileRotateOptions{
			MaxBytes:    cfg.RotationMaxSize,
			BackupCount: int(cfg.RotationMaxFiles),
			Path:        cfg.Output,
		})
		if err != nil {
			fmt.Printf("Error opening log output file: %s. Disabling logs!\n", err.Error())
			mainWriter = ioutil.Discard
		} else {
			fmt.Printf("Log file: %s \n", cfg.Output)
		}
	}

	nonDebugWriter := mainWriter
	_, err = url.ParseRequestURI(slackCfg.Webhook)
	if err == nil && slackCfg.Channel != "" {
		nonDebugWriter = io.MultiWriter(mainWriter, NewSlackWriter(slackCfg.Webhook, slackCfg.Channel))
	}

	var level int
	switch strings.ToUpper(cfg.Level) {
	case "VERBOSE":
		level = logging.LevelVerbose
	case "DEBUG":
		level = logging.LevelDebug
	case "INFO":
		level = logging.LevelInfo
	case "WARNING", "WARN":
		level = logging.LevelError
	case "ERROR":
		level = logging.LevelWarning
	case "NONE":
		level = logging.LevelNone
	}

	// buffer error, warning & info. don't buffer debug and verbose
	buffered := [5]bool{true, true, true, false, false}
	return NewHistoricLoggerWrapper(logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: log.Ldate | log.Ltime | log.Lshortfile,
		Prefix:              prefix,
		VerboseWriter:       mainWriter,
		DebugWriter:         mainWriter,
		InfoWriter:          nonDebugWriter,
		WarningWriter:       nonDebugWriter,
		ErrorWriter:         nonDebugWriter,
		LogLevel:            level,
		ExtraFramesToSkip:   1,
	}), buffered, 5)
}
