package log

import (
	"io"
	"io/ioutil"
	"log"
)

var (
	// Verbose level
	Verbose *log.Logger
	// Debug level
	Debug *log.Logger
	// Info level
	Info *log.Logger
	// Warning level
	Warning *log.Logger
	// Error level
	Error *log.Logger
)

// Initialize log module
func Initialize(logHandle io.Writer, debug bool, verbose bool) {

	verboseHandle := ioutil.Discard
	if verbose {
		verboseHandle = logHandle
	}

	debugHandle := ioutil.Discard
	if debug {
		debugHandle = logHandle
	}

	Verbose = log.New(verboseHandle,
		"SPLITIO-AGENT | VERBOSE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Debug = log.New(debugHandle,
		"SPLITIO-AGENT | DEBUG: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(logHandle,
		"SPLITIO-AGENT | INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(logHandle,
		"SPLITIO-AGENT | WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(logHandle,
		"SPLITIO-AGENT | ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}
