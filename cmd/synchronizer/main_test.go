package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common"

	loggingMocks "github.com/splitio/go-toolkit/v5/logging/mocks"

	"github.com/stretchr/testify/assert"
)

func captureLogger(captured *[]string) *loggingMocks.MockLogger {
	return &loggingMocks.MockLogger{
		ErrorCall: func(msg ...interface{}) { *captured = append(*captured, fmt.Sprint(msg...)) },
	}
}

func TestHandleStartErrorInitializationError(t *testing.T) {
	var captured []string
	initErr := common.NewInitError(errors.New("something failed"), common.ExitRedisInitializationFailed)

	code := handleStartError(captureLogger(&captured), initErr)

	assert.Equal(t, initErr.ExitCode(), code)
	assert.Len(t, captured, 1)
	assert.Contains(t, captured[0], "split sync")
	assert.NotContains(t, captured[0], "split proxy")
	assert.Contains(t, captured[0], "something failed")
}

func TestHandleStartErrorWrappedInitializationError(t *testing.T) {
	var captured []string
	initErr := common.NewInitError(errors.New("something failed"), common.ExitTLSError)
	wrapped := fmt.Errorf("wrapping: %w", initErr)

	code := handleStartError(captureLogger(&captured), wrapped)

	assert.Equal(t, initErr.ExitCode(), code)
	assert.Len(t, captured, 1)
	assert.Contains(t, captured[0], "split sync")
	assert.Contains(t, captured[0], "something failed")
}

func TestHandleStartErrorUnexpectedError(t *testing.T) {
	var captured []string

	code := handleStartError(captureLogger(&captured), errors.New("boom"))

	assert.Equal(t, common.ExitUndefined, code)
	assert.Len(t, captured, 1)
	assert.Contains(t, captured[0], "split sync")
	assert.NotContains(t, captured[0], "split proxy")
	assert.Contains(t, captured[0], "boom")
}
