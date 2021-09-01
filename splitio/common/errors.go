package common

import (
	"fmt"
)

// Exit codes
const (
	ExitSuccess = iota
	ExitInvalidApikey
	ExitInvalidConfiguration
	ExitRedisInitializationFailed
	ExitErrorDB
	ExitTaskInitialization
	ExitAdminError
	ExitUndefined
)

// InitializationError wraps an error and an exit code
type InitializationError struct {
	err      error
	exitCode int
}

// NewInitError constructs an initialization error
func NewInitError(err error, exitCode int) *InitializationError {
	return &InitializationError{
		err:      err,
		exitCode: exitCode,
	}
}

// Error returns the string representation of the error causing the initialization failure
func (e *InitializationError) Error() string {
	return fmt.Sprintf("initialization error: %s", e.err)
}

// ExitCode is the number to return to the OS
func (e *InitializationError) ExitCode() int {
	return e.exitCode
}
