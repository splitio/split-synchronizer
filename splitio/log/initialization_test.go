package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"

	"github.com/stretchr/testify/assert"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	assert.Nil(t, err)
	return string(contents)
}

// emitAll writes one message per level and returns the contents of the log file once `expectedLines` have been flushed.
// (the file writer is asynchronous, so we wait for the expected lines and then a little longer to catch unexpected ones)
func emitAll(t *testing.T, level string, expectedLines int) (string, *HistoricLoggerWrapper) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	cfg := &conf.Logging{Level: level, Output: path, RotationMaxFiles: 1, RotationMaxSizeKb: 1024}

	logger := BuildFromConfig(cfg, "test", &conf.Slack{})
	logger.Error("msg-error")
	logger.Warning("msg-warning")
	logger.Info("msg-info")
	logger.Debug("msg-debug")
	logger.Verbose("msg-verbose")

	assert.Eventually(t, func() bool {
		return strings.Count(readLog(t, path), "\n") >= expectedLines
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	return readLog(t, path), logger
}

func assertEmitted(t *testing.T, contents string, emitted []string, suppressed []string) {
	t.Helper()
	for _, m := range emitted {
		assert.Contains(t, contents, m)
	}
	for _, m := range suppressed {
		assert.NotContains(t, contents, m)
	}
}

func TestBuildFromConfigLevels(t *testing.T) {
	all := []string{"msg-error", "msg-warning", "msg-info", "msg-debug", "msg-verbose"}
	tests := []struct {
		level   string
		emitted int // number of messages (in severity order) that should be emitted
	}{
		{"ERROR", 1},
		{"error", 1},
		{"WARN", 2},
		{"WARNING", 2},
		{"warning", 2},
		{"INFO", 3},
		{"info", 3},
		{"DEBUG", 4},
		{"VERBOSE", 5},
		{"verbose", 5},
		{"NONE", 0},
		{"none", 0},
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			contents, _ := emitAll(t, tc.level, tc.emitted)
			assertEmitted(t, contents, all[:tc.emitted], all[tc.emitted:])
			assert.NotContains(t, contents, "Unrecognized log level")
		})
	}
}

func TestBuildFromConfigUnrecognizedLevelDefaultsToInfo(t *testing.T) {
	for _, level := range []string{"TRACE", ""} {
		t.Run("level="+level, func(t *testing.T) {
			contents, logger := emitAll(t, level, 4) // error, warning, info + the unrecognized level warning
			assertEmitted(t, contents,
				[]string{"msg-error", "msg-warning", "msg-info"},
				[]string{"msg-debug", "msg-verbose"})

			assert.Contains(t, contents, "Unrecognized log level")
			assert.Contains(t, contents, `"`+level+`"`)
			assert.Contains(t, contents, "Defaulting to INFO")

			// the warning also reaches the historic buffer
			found := false
			for _, m := range logger.Messages(logging.LevelWarning) {
				found = found || strings.Contains(m, "Unrecognized log level")
			}
			assert.True(t, found, "warning should be recorded in the historic buffer")
		})
	}
}

func TestBuildFromConfigNilSlack(t *testing.T) {
	cfg := &conf.Logging{Level: "info", Output: filepath.Join(t.TempDir(), "test.log"), RotationMaxFiles: 1, RotationMaxSizeKb: 1024}
	assert.NotPanics(t, func() { BuildFromConfig(cfg, "test", nil) })
}
