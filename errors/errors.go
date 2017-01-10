// Package errors implements functions to frequent use
package errors

import "github.com/splitio/go-agent/iohelper"

// CheckError checks the error status given an error object.
func CheckError(err error) {
	if err != nil {
		iohelper.PrintlnError(err, "Checking error function found this error ")
	}
}

// IsError returns true if err != nil
func IsError(err error) bool {
	if err != nil {
		return true
	}
	return false
}
