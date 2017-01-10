// Package iohelper implements input/output functions to frequent use
package iohelper

import (
	"fmt"
	"strings"
)

const outputMarker = "--> "

// Println print a message to standard output
func Println(messages ...string) {
	fmt.Println(outputMarker, strings.Join(messages, " "))
}

// PrintlnError print a message including an Error object to standard output
func PrintlnError(err error, messages ...string) {
	fmt.Println(outputMarker, strings.Join(messages, " "), err)
}
